// Command s3disk mounts an S3 bucket as a POSIX filesystem.
//
//	s3disk mount s3://bucket/prefix /mnt/project
//	s3disk status /mnt/project
//	s3disk sync   /mnt/project
//	s3disk umount /mnt/project
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
)

// version is stamped at build time with -ldflags "-X main.version=...".
// It is empty for a binary produced by `go install module@version`, which
// cannot pass ldflags — resolvedVersion falls back to the module version Go
// records in the build info, so an installed binary still reports what it is.
var version = ""

// resolvedVersion reports the build's version, preferring an explicit stamp.
func resolvedVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}

func main() {
	// Support being invoked as a mount(8) helper:
	//   mount -t fuse.s3disk bucket:prefix /mnt -o ro,endpoint=...
	if base := filepath.Base(os.Args[0]); base == "mount.s3disk" || base == "mount.fuse.s3disk" {
		os.Exit(runMountHelper(os.Args[1:]))
	}
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	var code int
	switch cmd {
	case "mount":
		code = runMount(args)
	case "umount", "unmount":
		code = runUmount(args)
	case "status":
		code = runStatus(args)
	case "sync":
		code = runSync(args)
	case "refresh":
		code = runRefresh(args)
	case "list", "ls":
		code = runList(args)
	case "doctor":
		code = runDoctor(args)
	case "version", "--version", "-v":
		fmt.Printf("s3disk %s\n", resolvedVersion())
	case "help", "--help", "-h":
		usage()
	default:
		// `s3disk s3://bucket /mnt` is accepted as shorthand for `mount`.
		if strings.HasPrefix(cmd, "s3://") {
			code = runMount(os.Args[1:])
		} else {
			fmt.Fprintf(os.Stderr, "s3disk: unknown command %q\n\n", cmd)
			usage()
			code = 2
		}
	}
	os.Exit(code)
}

func usage() {
	fmt.Fprint(os.Stderr, `s3disk — mount an S3 bucket as a normal filesystem

Usage:
  s3disk mount  s3://BUCKET[/PREFIX] MOUNTPOINT [options]
  s3disk umount MOUNTPOINT
  s3disk status [MOUNTPOINT]        show cache, dirty files and S3 request counts
  s3disk sync   [MOUNTPOINT]        flush every pending write to S3 now
  s3disk refresh [MOUNTPOINT]       forget cached metadata after an out-of-band change
  s3disk list                       list live s3disk mounts
  s3disk doctor [s3://BUCKET]       check FUSE, credentials and bucket access
  s3disk version

Run "s3disk mount -h" for the full option list.

Examples:
  s3disk mount s3://my-bucket/project /workspace
  s3disk mount s3://my-bucket /data --endpoint http://minio:9000 --path-style
  s3disk mount s3://my-bucket /data --read-only --attr-mode fast
`)
}

func fatalf(format string, args ...any) int {
	fmt.Fprintf(os.Stderr, "s3disk: "+format+"\n", args...)
	return 1
}
