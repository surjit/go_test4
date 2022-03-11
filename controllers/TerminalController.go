package controllers

import (
	"baljeet/cmd"
	"encoding/json"
	"fmt"
	"github.com/creack/pty"
	"github.com/gofiber/fiber/v2"
	"io"
	"log"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/gofiber/websocket/v2"
)

type Event string

const (
	EventResize  Event = "resize"
	EventSendkey Event = "sendKey"
	EventClose   Event = "close"
)

type Message struct {
	Event Event
	Data  interface{}
}

type wsConn struct {
	conn *websocket.Conn
	buf  []byte
}

// run command
var command = cmd.Getenv("SHELL", "bash")

type client struct{} // Add more data to this type if needed

var clients = make(map[*websocket.Conn]client) // Note: although large maps with pointer-like types (e.g. strings) as keys are slow, using pointers themselves as keys is acceptable and fast
var register = make(chan *websocket.Conn)
var broadcast = make(chan string)
var unregister = make(chan *websocket.Conn)

var ptmx *os.File
var execCmd *exec.Cmd

// wait time
var checkProcInterval = 5

func RunHub() {
	for {
		select {
		case connection := <-register:
			clients[connection] = client{}
			log.Println("connection registered")

		case message := <-broadcast:
			log.Println("message received:", message)

			// Send the message to all clients
			for ws := range clients {
				defer ws.Close()

				wsconn := &wsConn{
					conn: ws,
				}

				availableShell := make(map[int]string)
				availableShell[1] = "/bin/sh"
				availableShell[2] = "/bin/bash"
				availableShell[3] = "/usr/bin/bash"
				availableShell[4] = "/bin/dash"
				availableShell[5] = "/usr/bin/dash"
				availableShell[6] = "/bin/rbash"
				availableShell[7] = "/usr/bin/rbash"

				osUser := "root"
				Shell := "2"

				shellIndex, err := strconv.Atoi(Shell)
				if err != nil {
					// handle error
					_ = ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("%s\r\n", "Error: Invalid request")))
					return
				}

				sysUser, err0 := user.Lookup(osUser)

				if err0 != nil {
					_ = ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("%s\r\n", "Error: User does not found")))
					return
				}

				if osUser == "root" {
					command = "/usr/bin/bash"
				} else {
					command = availableShell[shellIndex]
				}

				if ptmx == nil {
					var msg Message
					// if err := ws.ReadJSON(&msg); err != nil {
					if err := json.Unmarshal([]byte(message), &msg); err != nil {
						log.Println("failed to decode message:", err)
						return
					}

					rows, cols, err := cmd.WindowSize(msg.Data)
					if err != nil {
						if err = ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("%s\r\n", err))); err != nil {
							log.Println("write:", err)
							return
						}
					}
					winsize := &pty.Winsize{
						Rows: rows,
						Cols: cols,
					}

					c := cmd.Filter(strings.Split(command, " "))
					if len(c) > 1 {
						//nolint
						execCmd = exec.Command(c[0], c[1:]...)
					} else {
						//nolint
						execCmd = exec.Command(c[0])
					}

					Uid, _ := strconv.ParseUint(sysUser.Gid, 10, 32)
					Gid, _ := strconv.ParseUint(sysUser.Gid, 10, 32)

					execCmd.Dir = sysUser.HomeDir
					execCmd.Env = append(os.Environ(), "USER="+osUser, "HOME="+sysUser.HomeDir)
					execCmd.SysProcAttr = &syscall.SysProcAttr{}
					execCmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(Uid), Gid: uint32(Gid)}

					ptmx, err = pty.StartWithSize(execCmd, winsize)
					if err != nil {
						log.Println("failed to create pty", err)
						return
					}
				}

				// write data to process
				go func() {
					for {
						var msg Message
						//if err := ws.ReadJSON(&msg); err != nil {
						if err := json.Unmarshal([]byte(message), &msg); err != nil {
							log.Println("websocket closed")

							if execCmd != nil {
								_ = execCmd.Process.Kill()
								_, _ = execCmd.Process.Wait()
							}

							// close current session, when browser terminal is closed
							ptmx.Close()
							ptmx = nil
							execCmd = nil
							return
						}

						if msg.Event == EventClose {
							log.Println("close websocket")
							ws.Close()
							return
						}

						if msg.Event == EventResize {
							rows, cols, err := cmd.WindowSize(msg.Data)
							if err != nil {
								log.Println(err)
								return
							}

							winsize := &pty.Winsize{
								Rows: rows,
								Cols: cols,
							}

							if err := pty.Setsize(ptmx, winsize); err != nil {
								log.Println("failed to set window size:", err)
								return
							}
							continue
						}

						data, ok := msg.Data.(string)
						if !ok {
							log.Println("invalid message data:", data)
							return
						}

						_, err := ptmx.WriteString(data)
						if err != nil {
							log.Println("failed to write data to ptmx:", err)
							return
						}
					}
				}()

				_, _ = io.Copy(wsconn, ptmx)
				delete(clients, ws)
			}

			// check process state
			go func() {
				ticker := time.NewTicker(time.Duration(checkProcInterval) * time.Second)
				for range ticker.C {
					if execCmd != nil {
						state, err := execCmd.Process.Wait()
						if err != nil {
							return
						}

						if state.ExitCode() != -1 {
							ptmx.Close()
							ptmx = nil
							execCmd = nil
						}
					}
				}
			}()

		case connection := <-unregister:
			// Remove the client from the hub

			if execCmd != nil {
				_ = execCmd.Process.Kill()
				_, _ = execCmd.Process.Wait()
			}

			// close current session, when browser terminal is closed
			ptmx.Close()
			ptmx = nil
			execCmd = nil

			delete(clients, connection)
			log.Println("connection unregistered")
		}
	}
}

// Checking and buffering `b`
// If `b` is invalid UTF-8, it would be buffered
// if buffer is valid UTF-8, it would write to connection
func (ws *wsConn) Write(b []byte) (i int, err error) {
	if !utf8.Valid(b) {
		buflen := len(ws.buf)
		blen := len(b)
		ws.buf = append(ws.buf, b...)[:buflen+blen]
		if utf8.Valid(ws.buf) {
			e := ws.conn.WriteMessage(websocket.TextMessage, ws.buf)
			ws.buf = ws.buf[:0]
			return blen, e
		}
		return blen, nil
	}

	blen0 := len(b)

	if len(ws.buf) > 0 {
		err := ws.conn.WriteMessage(websocket.TextMessage, ws.buf)
		ws.buf = ws.buf[:0]
		if err != nil {
			return blen0, nil
		}
	}

	e := ws.conn.WriteMessage(websocket.TextMessage, b)

	return blen0, e
}

func TerminalHandler() fiber.Handler {
	return websocket.New(func(c *websocket.Conn) {
		// When the function returns, unregister the client and close the connection
		defer func() {
			unregister <- c
			c.Close()
		}()

		// Register the client
		register <- c

		for {
			messageType, message, err := c.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Println("read error:", err)
				}

				return // Calls the deferred function, i.e. closes the connection on error
			}

			if messageType == websocket.TextMessage {
				// Broadcast the received message
				broadcast <- string(message)
			} else {
				log.Println("websocket message received of type", messageType)
			}
		}
	})
}
