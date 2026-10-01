package main

import (
	"crypto/rand"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
)

func main() {
	if os.Getpid() == 1 {
		c := make(chan os.Signal, 4)
		signal.Notify(c, syscall.SIGTERM, syscall.SIGINT, syscall.SIGCHLD)
		for s := range c {
			if s != syscall.SIGCHLD {
				return
			}
			for {
				p, _ := syscall.Wait4(-1, nil, syscall.WNOHANG, nil)
				if p <= 0 {
					break
				}
			}
		}
		return
	}
	if len(os.Args) < 3 {
		os.Exit(2)
	}
	var e error
	switch os.Args[1] {
	case "write":
		e = os.WriteFile(os.Args[2], []byte(os.Args[3]), 0644)
	case "read":
		var b []byte
		b, e = os.ReadFile(os.Args[2])
		fmt.Print(string(b))
	case "fill":
		var f *os.File
		f, e = os.Create(os.Args[2])
		if e == nil {
			b := make([]byte, 1024*1024)
			rand.Read(b)
			n, _ := strconv.Atoi(os.Args[3])
			for i := 0; i < n; i++ {
				_, e = f.Write(b)
				if e != nil {
					break
				}
				e = f.Sync()
				if e != nil {
					break
				}
			}
			f.Close()
		}
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
