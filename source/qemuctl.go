package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

type qemuController struct {
	proc   *exec.Cmd
	qmp    net.Conn
	output chan string
	done   chan struct{}
}

func calcGeometry(vhdPath string) (cyls, heads, spt int, err error) {
	fi, err := os.Stat(vhdPath)
	if err != nil {
		return 0, 0, 0, err
	}
	totalSectors := fi.Size() / 512
	heads, spt = geomHeads, geomSPT
	cyls = int(totalSectors / int64(heads*spt))
	return cyls, heads, spt, nil
}

func (c *qemuController) start(vhd, floppy, bootref string) error {
	_ = os.Remove(qmpSock)

	cyls, heads, spt, err := calcGeometry(vhd)
	if err != nil {
		return err
	}
	eprintf("[*] VHD geometry: %d/%d/%d\n", cyls, heads, spt)

	args := []string{
		"-L", qemuBiosDir,
		"-nographic",
		"-no-reboot",
		"-m", "32",
		"-drive", fmt.Sprintf("file=%s,format=raw,if=floppy", floppy),
		"-drive", fmt.Sprintf("file=%s,format=raw,if=none,id=maindisk,media=disk", vhd),
		"-device", fmt.Sprintf("ide-hd,drive=maindisk,bus=ide.0,unit=0,cyls=%d,heads=%d,secs=%d", cyls, heads, spt),
		"-drive", fmt.Sprintf("file=%s,format=raw,if=none,id=bootref,media=disk", bootref),
		"-device", "ide-hd,drive=bootref,bus=ide.0,unit=1",
		"-boot", "order=a",
		"-qmp", fmt.Sprintf("unix:%s,server,nowait", qmpSock),
		"-serial", "stdio",
		"-monitor", "none",
	}
	eprintf("[*] Starting QEMU...\n")

	c.proc = exec.Command(qemuBin, args...)
	stdout, err := c.proc.StdoutPipe()
	if err != nil {
		return err
	}
	c.proc.Stderr = c.proc.Stdout
	c.proc.Stdin = nil
	if err := c.proc.Start(); err != nil {
		return err
	}

	c.output = make(chan string, 256)
	c.done = make(chan struct{})
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				text := string(buf[:n])
				fmt.Fprint(os.Stderr, text)
				select {
				case c.output <- text:
				default:
				}
			}
			if err != nil {
				close(c.done)
				return
			}
		}
	}()
	return nil
}

func (c *qemuController) connectQMP(timeout time.Duration) bool {
	eprintf("[*] Connecting to QMP...\n")
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("unix", qmpSock, 2*time.Second)
		if err == nil {
			buf := make([]byte, 4096)
			_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			_, _ = conn.Read(buf)
			cap, _ := json.Marshal(map[string]string{"execute": "qmp_capabilities"})
			_, _ = conn.Write(append(cap, '\n'))
			_, _ = conn.Read(buf)
			c.qmp = conn
			eprintf("[*] QMP connected\n")
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

var qemuKeymap = map[rune]struct {
	key   string
	shift bool
}{
	'\n': {"ret", false}, '\r': {"ret", false},
	' ': {"spc", false}, '\\': {"backslash", false},
	'/': {"slash", false}, ':': {"semicolon", true},
	'.': {"dot", false}, '-': {"minus", false},
}

func (c *qemuController) sendKey(key string, shift bool) {
	type kevent struct {
		Type string `json:"type"`
		Data struct {
			Down bool `json:"down"`
			Key  struct {
				Type string `json:"type"`
				Data string `json:"data"`
			} `json:"key"`
		} `json:"data"`
	}
	mk := func(k string, down bool) kevent {
		var e kevent
		e.Type = "key"
		e.Data.Down = down
		e.Data.Key.Type = "qcode"
		e.Data.Key.Data = k
		return e
	}
	var events []kevent
	if shift {
		events = append(events, mk("shift", true))
	}
	events = append(events, mk(key, true), mk(key, false))
	if shift {
		events = append(events, mk("shift", false))
	}
	payload := map[string]interface{}{
		"execute":   "input-send-event",
		"arguments": map[string]interface{}{"events": events},
	}
	b, _ := json.Marshal(payload)
	if c.qmp != nil {
		_, _ = c.qmp.Write(append(b, '\n'))
	}
	sleep(0.15)
}

func (c *qemuController) sendString(text string) {
	for _, ch := range text {
		var key string
		var shift bool
		if km, ok := qemuKeymap[ch]; ok {
			key, shift = km.key, km.shift
		} else if ch >= 'A' && ch <= 'Z' {
			key, shift = strings.ToLower(string(ch)), true
		} else if ch >= '0' && ch <= '9' {
			key, shift = string(ch), false
		} else {
			key, shift = strings.ToLower(string(ch)), false
		}
		c.sendKey(key, shift)
	}
}

func (c *qemuController) waitFor(patterns []string, timeout time.Duration, window int) string {
	deadline := time.Now().Add(timeout)
	acc := ""
	for time.Now().Before(deadline) {
		select {
		case chunk := <-c.output:
			acc += chunk
			if len(acc) > window {
				acc = acc[len(acc)-window:]
			}
			lowerAcc := strings.ToLower(acc)
			for _, p := range patterns {
				if strings.Contains(lowerAcc, strings.ToLower(p)) {
					return p
				}
			}
		case <-c.done:
			eprintf("[*] QEMU exited\n")
			return "QEMU_EXITED"
		case <-time.After(2 * time.Second):
		}
	}
	return ""
}

func (c *qemuController) shutdown() {
	if c.qmp != nil {
		quit, _ := json.Marshal(map[string]string{"execute": "quit"})
		_, _ = c.qmp.Write(append(quit, '\n'))
	}
	if c.proc != nil && c.proc.Process != nil {
		waitCh := make(chan error, 1)
		go func() { waitCh <- c.proc.Wait() }()
		select {
		case <-waitCh:
		case <-time.After(10 * time.Second):
			_ = c.proc.Process.Kill()
		}
	}
}

func runDOSInstall(vhd, floppy, bootref string) bool {
	ctrl := &qemuController{}
	if err := ctrl.start(vhd, floppy, bootref); err != nil {
		eprintf("[!] Failed to start QEMU: %v\n", err)
		return false
	}
	if !ctrl.connectQMP(60 * time.Second) {
		eprintf("[!] Failed to connect to QMP\n")
		ctrl.shutdown()
		return false
	}

	eprintf("[*] Waiting for FORMAT confirmation prompt...\n")
	result := ctrl.waitFor([]string{"Proceed with Format", "Invalid drive", "QEMU_EXITED"}, 120*time.Second, 4000)
	if result == "" {
		eprintf("[!] Timed out waiting for FORMAT prompt\n")
		ctrl.shutdown()
		return false
	}
	if result == "QEMU_EXITED" {
		eprintf("[!] QEMU exited unexpectedly before reaching FORMAT prompt\n")
		return false
	}
	if strings.Contains(strings.ToLower(result), "invalid drive") {
		eprintf("[!] DOS cannot see the VHD - geometry issue persists\n")
		ctrl.shutdown()
		return false
	}

	eprintf("[*] Sending Y to FORMAT...\n")
	sleep(1)
	ctrl.sendString("Y\n")

	eprintf("[*] Waiting for setup completion...\n")
	result = ctrl.waitFor([]string{
		"AO486 SETUP COMPLETE", "SETUP COMPLETE DEBUG", "SETUP COMPLETE", "QEMU_EXITED",
	}, 300*time.Second, 4000)

	sleep(2)
	ctrl.shutdown()

	if result == "" {
		eprintf("[!] Timed out waiting for setup completion\n")
		return false
	}
	if result == "QEMU_EXITED" {
		eprintf("[!] QEMU exited before setup completed\n")
		return false
	}
	return true
}
