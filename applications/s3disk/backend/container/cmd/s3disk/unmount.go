package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"futrx.local/catalog/applications/s3disk/backend/container/internal/ctl"
	"futrx.local/catalog/applications/s3disk/backend/container/internal/s3fs"
)

// runUmount flushes pending writes and detaches the mount.
func runUmount(args []string) int {
	fs := flag.NewFlagSet("umount", flag.ContinueOnError)
	force := fs.Bool("force", false, "detach even if the mount is busy (lazy unmount)")
	noSync := fs.Bool("no-sync", false, "skip flushing pending writes first")
	if err := fs.Parse(permute(fs, args)); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: s3disk umount MOUNTPOINT [--force] [--no-sync]")
		return 2
	}
	mp, err := filepath.Abs(fs.Arg(0))
	if err != nil {
		return fatalf("%v", err)
	}

	if !*noSync {
		if entry, ok := ctl.Find(mp); ok {
			if _, err := ask(entry, "/sync", 30*time.Minute); err != nil {
				fmt.Fprintf(os.Stderr, "s3disk: warning: could not flush before unmount: %v\n", err)
			}
		}
	}

	if err := unmountPath(mp, *force); err != nil {
		return fatalf("%v", err)
	}
	// The serving process tears itself down once the kernel drops the mount.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if mounted, err := s3fs.IsMounted(mp); err == nil && !mounted {
			fmt.Printf("unmounted %s\n", mp)
			return 0
		}
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Printf("unmount requested for %s\n", mp)
	return 0
}

func unmountPath(mp string, force bool) error {
	var lastErr error
	for _, attempt := range [][]string{
		{"fusermount3", "-u", mp},
		{"fusermount", "-u", mp},
		{"umount", mp},
	} {
		bin, err := exec.LookPath(attempt[0])
		if err != nil {
			continue
		}
		cmd := exec.Command(bin, attempt[1:]...)
		out, err := cmd.CombinedOutput()
		if err == nil {
			return nil
		}
		lastErr = fmt.Errorf("%s: %v: %s", attempt[0], err, strings.TrimSpace(string(out)))
	}
	if force {
		for _, attempt := range [][]string{
			{"fusermount3", "-uz", mp},
			{"umount", "-l", mp},
		} {
			bin, err := exec.LookPath(attempt[0])
			if err != nil {
				continue
			}
			if out, err := exec.Command(bin, attempt[1:]...).CombinedOutput(); err == nil {
				return nil
			} else {
				lastErr = fmt.Errorf("%s: %v: %s", attempt[0], err, strings.TrimSpace(string(out)))
			}
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no unmount helper found (install fuse3)")
	}
	return lastErr
}
