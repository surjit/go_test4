package controllers

import (
	"baljeet/cmd"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
	"os"
	"os/exec"
	"time"
	"unicode/utf8"
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

// wait time
var checkProcInterval = 5

func TerminalHandler() fiber.Handler {
	var ptmx *os.File
	var execCmd *exec.Cmd

	socket := websocket.New(func(ws *websocket.Conn) {
		defer ws.Close()

		Terminal(ws, ptmx, execCmd)
	})

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

	return socket
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
