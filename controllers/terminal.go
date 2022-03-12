package controllers

import (
	"baljeet/cmd"
	"fmt"
	"github.com/creack/pty"
	"github.com/gofiber/websocket/v2"
	"io"
	"log"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"
)

func Terminal(ws *websocket.Conn, ptmx *os.File, execCmd *exec.Cmd) {
	ws_conn := &wsConn{
		conn: ws,
	}

	ptmx = nil
	execCmd = nil

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
		if err := ws.ReadJSON(&msg); err != nil {
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

	go Goroute(ws, ptmx, execCmd)

	_, _ = io.Copy(ws_conn, ptmx)
}
