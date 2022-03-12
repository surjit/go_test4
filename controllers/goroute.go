package controllers

import (
	"baljeet/cmd"
	"github.com/creack/pty"
	"github.com/gofiber/websocket/v2"
	"log"
	"os"
	"os/exec"
)

func Goroute(ws *websocket.Conn, ptmx *os.File, execCmd *exec.Cmd) {
	// write data to process
	for {
		var msg Message
		if err := ws.ReadJSON(&msg); err != nil {
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
}
