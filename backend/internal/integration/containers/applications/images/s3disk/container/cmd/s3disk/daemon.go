package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"s3disk/internal/config"
)

// Mounting detaches by default, so the shell prompt comes back only once the
// filesystem works. This file owns that: the re-exec, the pipe the child
// answers on, and where its log goes once it no longer has the caller's
// stderr.

// readyFD is the pipe the daemonised child uses to tell its parent that the
// mount is up (or why it is not).
const readyFD = 3

// daemonize re-executes this binary in the background and waits for the child
// to report that the mount is ready, so the shell prompt returns only when the
// filesystem is actually usable.
func daemonize(cfg *config.Config, args []string) int {
	exe, err := os.Executable()
	if err != nil {
		return fatalf("%v", err)
	}
	r, w, err := os.Pipe()
	if err != nil {
		return fatalf("%v", err)
	}
	cmd := exec.Command(exe, append([]string{"mount"}, args...)...)
	cmd.Env = append(os.Environ(), "S3DISK_DAEMON=1")
	cmd.Stdin = nil
	cmd.Stdout = nil
	// The daemon must not hold the caller's stderr open: it outlives this
	// process, and anything capturing our output (a command substitution, a
	// pipeline) would block forever waiting for that descriptor to close.
	// Point it at the log file instead, so panics and stray output survive.
	if logFile := openDaemonLog(cfg); logFile != nil {
		cmd.Stderr = logFile
		defer logFile.Close()
	}
	cmd.ExtraFiles = []*os.File{w}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fatalf("%v", err)
	}
	w.Close()

	// The child writes either "READY" or the reason it could not start.
	msg, _ := io.ReadAll(r)
	text := strings.TrimSpace(string(msg))
	if text == readyMessage {
		return 0
	}
	if text != "" {
		fmt.Fprintln(os.Stderr, text)
	} else {
		fmt.Fprintf(os.Stderr, "s3disk: the mount process exited during startup; see %s\n", daemonLogPath(cfg))
	}
	_ = cmd.Wait()
	return 1
}

func daemonLogPath(cfg *config.Config) string {
	if cfg.LogFile != "" {
		return cfg.LogFile
	}
	return filepath.Join(cfg.CacheDir, "s3disk.log")
}

func openDaemonLog(cfg *config.Config) *os.File {
	path := daemonLogPath(cfg)
	if path == "-" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil
	}
	return f
}

// readyMessage is the token a daemonised child sends once the mount works.
const readyMessage = "READY"

// reportReady tells the parent process that the filesystem is usable.
func reportReady() {
	if os.Getenv("S3DISK_DAEMON") == "" {
		return
	}
	if pipe := os.NewFile(readyFD, "ready"); pipe != nil {
		fmt.Fprint(pipe, readyMessage)
		pipe.Close()
	}
}

// startupError reports a failure that happened before the mount came up. When
// daemonised the message travels back over the ready pipe, so the user sees the
// real reason on their terminal rather than only in a log file.
func startupError(err error) int {
	msg := fmt.Sprintf("s3disk: %v", err)
	if os.Getenv("S3DISK_DAEMON") != "" {
		if pipe := os.NewFile(readyFD, "ready"); pipe != nil {
			fmt.Fprint(pipe, msg)
			pipe.Close()
		}
	}
	fmt.Fprintln(os.Stderr, msg)
	return 1
}

func setupLogging(cfg *config.Config) (*log.Logger, func(), error) {
	out := io.Writer(os.Stderr)
	closer := func() {}
	path := cfg.LogFile
	if path == "" && os.Getenv("S3DISK_DAEMON") != "" {
		if err := os.MkdirAll(cfg.CacheDir, 0700); err == nil {
			path = filepath.Join(cfg.CacheDir, "s3disk.log")
		}
	}
	if path != "" && path != "-" {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return nil, nil, fmt.Errorf("opening log file %s: %w", path, err)
		}
		out = f
		closer = func() { f.Close() }
	}
	return log.New(out, "s3disk: ", log.LstdFlags|log.Lmsgprefix), closer, nil
}
