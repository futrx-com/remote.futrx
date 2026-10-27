//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

// startInPTY starts cmd as the session leader of a new pseudo-terminal and
// returns the master side. Closing the master hangs up the terminal.
func startInPTY(cmd *exec.Cmd) (*os.File, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	var unlock int32
	if err := ioctl(master, syscall.TIOCSPTLCK, unsafe.Pointer(&unlock)); err != nil {
		master.Close()
		return nil, fmt.Errorf("unlock pty: %w", err)
	}
	var number uint32
	if err := ioctl(master, syscall.TIOCGPTN, unsafe.Pointer(&number)); err != nil {
		master.Close()
		return nil, fmt.Errorf("name pty: %w", err)
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		return nil, err
	}
	defer slave.Close()

	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	// Ctty indexes the child's descriptors, where stdin is 0.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		master.Close()
		return nil, err
	}
	return master, nil
}

type winsize struct {
	Rows, Cols, X, Y uint16
}

func setWindowSize(master *os.File, cols, rows uint16) error {
	size := winsize{Rows: rows, Cols: cols}
	return ioctl(master, syscall.TIOCSWINSZ, unsafe.Pointer(&size))
}

func ioctl(file *os.File, request uintptr, argument unsafe.Pointer) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), request, uintptr(argument)); errno != 0 {
		return errno
	}
	return nil
}
