package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"s3disk/internal/config"
	"s3disk/internal/ctl"
	"s3disk/internal/s3fs"
	"s3disk/internal/s3io"
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
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()
			if _, err := ctl.NewClient(entry.Socket).Get(ctx, "/sync"); err != nil {
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

// runStatus prints a mount's cache, dirty files and S3 request counters.
func runStatus(args []string) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print raw JSON")
	if err := fs.Parse(permute(fs, args)); err != nil {
		return 2
	}
	entry, ok := ctl.Find(fs.Arg(0))
	if !ok {
		if fs.Arg(0) == "" {
			return fatalf("no s3disk mount found (or more than one; pass a mountpoint)")
		}
		return fatalf("no s3disk mount at %s", fs.Arg(0))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	body, err := ctl.NewClient(entry.Socket).Get(ctx, "/status")
	if err != nil {
		return fatalf("contacting mount: %v", err)
	}
	if *asJSON {
		fmt.Println(strings.TrimSpace(string(body)))
		return 0
	}
	var st s3fs.Status
	if err := json.Unmarshal(body, &st); err != nil {
		fmt.Println(string(body))
		return 0
	}
	fmt.Printf("mountpoint   %s\n", st.Mountpoint)
	fmt.Printf("source       s3://%s/%s\n", st.Bucket, st.Prefix)
	fmt.Printf("cache dir    %s\n", st.CacheDir)
	fmt.Printf("uptime       %s", st.Uptime)
	if st.ReadOnly {
		fmt.Print("   (read-only)")
	}
	if st.Exclusive {
		fmt.Print("   (exclusive: sole writer)")
	}
	fmt.Printf("\n\n")
	fmt.Printf("cache        %s of %s in %d files, %d dirty\n",
		humanBytes(st.Cache.Bytes), humanBytes(st.Cache.MaxBytes), st.Cache.Entries, st.Cache.Dirty)
	fmt.Printf("             %d hits, %d misses, %d evictions, %d uploads\n",
		st.Cache.Hits, st.Cache.Misses, st.Cache.Evictions, st.Cache.Uploads)
	fmt.Printf("s3 requests  %d HEAD  %d LIST  %d GET  %d PUT  %d COPY  %d DELETE  %d errors\n",
		st.S3.Heads, st.S3.Lists, st.S3.Gets, st.S3.Puts, st.S3.Copies, st.S3.Deletes, st.S3.Errors)
	fmt.Printf("transferred  %s down, %s up\n", humanBytes(st.S3.BytesDown), humanBytes(st.S3.BytesUp))
	if len(st.DirtyFiles) > 0 {
		fmt.Printf("\npending upload (%d):\n", len(st.DirtyFiles))
		for i, k := range st.DirtyFiles {
			if i == 20 {
				fmt.Printf("  … and %d more\n", len(st.DirtyFiles)-20)
				break
			}
			fmt.Printf("  %s\n", k)
		}
	}
	return 0
}

// runSync forces every pending write to S3 and waits for it to land.
func runSync(args []string) int {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	if err := fs.Parse(permute(fs, args)); err != nil {
		return 2
	}
	entry, ok := ctl.Find(fs.Arg(0))
	if !ok {
		return fatalf("no s3disk mount found at %q", fs.Arg(0))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()
	body, err := ctl.NewClient(entry.Socket).Get(ctx, "/sync")
	if err != nil {
		return fatalf("contacting mount: %v", err)
	}
	var reply map[string]string
	_ = json.Unmarshal(body, &reply)
	if e, ok := reply["error"]; ok {
		return fatalf("%s", e)
	}
	fmt.Printf("flushed %s to s3://%s/%s\n", entry.Mountpoint, entry.Bucket, entry.Prefix)
	return 0
}

// runRefresh drops cached metadata so out-of-band changes to the bucket show up.
func runRefresh(args []string) int {
	fs := flag.NewFlagSet("refresh", flag.ContinueOnError)
	if err := fs.Parse(permute(fs, args)); err != nil {
		return 2
	}
	entry, ok := ctl.Find(fs.Arg(0))
	if !ok {
		return fatalf("no s3disk mount found at %q", fs.Arg(0))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if _, err := ctl.NewClient(entry.Socket).Get(ctx, "/refresh"); err != nil {
		return fatalf("contacting mount: %v", err)
	}
	fmt.Printf("refreshed %s; the next access re-reads from s3://%s/%s\n",
		entry.Mountpoint, entry.Bucket, entry.Prefix)
	return 0
}

// runList shows every live mount.
func runList(args []string) int {
	entries := ctl.List()
	if len(entries) == 0 {
		fmt.Println("no s3disk mounts")
		return 0
	}
	fmt.Printf("%-28s %-32s %-8s %s\n", "MOUNTPOINT", "SOURCE", "PID", "UPTIME")
	for _, e := range entries {
		src := "s3://" + e.Bucket
		if e.Prefix != "" {
			src += "/" + strings.TrimSuffix(e.Prefix, "/")
		}
		fmt.Printf("%-28s %-32s %-8d %s\n", e.Mountpoint, src, e.PID,
			time.Since(e.Started).Round(time.Second))
	}
	return 0
}

// runDoctor checks everything that commonly goes wrong before a first mount.
func runDoctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	endpoint := fs.String("endpoint", envOr("S3DISK_ENDPOINT", envOr("AWS_ENDPOINT_URL", "")), "S3 endpoint URL")
	region := fs.String("region", envOr("AWS_REGION", "us-east-1"), "S3 region")
	pathStyle := fs.Bool("path-style", envBool("S3DISK_PATH_STYLE", false), "use path-style addressing")
	if err := fs.Parse(permute(fs, args)); err != nil {
		return 2
	}

	failures := 0
	check := func(name string, err error, hint string) {
		if err == nil {
			fmt.Printf("  ok    %s\n", name)
			return
		}
		failures++
		fmt.Printf("  FAIL  %s: %v\n", name, err)
		if hint != "" {
			fmt.Printf("        → %s\n", hint)
		}
	}

	fmt.Println("s3disk doctor")
	fmt.Println()
	fmt.Println("fuse:")
	_, devErr := os.Stat("/dev/fuse")
	check("/dev/fuse present", devErr,
		"in Docker, run with --device /dev/fuse --cap-add SYS_ADMIN")
	if devErr == nil {
		f, err := os.OpenFile("/dev/fuse", os.O_RDWR, 0)
		check("/dev/fuse writable", err, "check container permissions or run as root")
		if err == nil {
			f.Close()
		}
	}
	_, err := exec.LookPath("fusermount3")
	if err != nil {
		_, err = exec.LookPath("fusermount")
	}
	check("fusermount available", err, "install the fuse3 package (only needed for unprivileged unmounts)")

	fmt.Println()
	fmt.Println("credentials:")
	hasKeys := os.Getenv("AWS_ACCESS_KEY_ID") != "" && os.Getenv("AWS_SECRET_ACCESS_KEY") != ""
	if hasKeys {
		fmt.Println("  ok    AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY set")
	} else {
		fmt.Println("  info  no static keys in the environment; the AWS default chain will be used")
		fmt.Println("        (shared profile, container/instance role, or web identity)")
	}

	if fs.NArg() > 0 {
		bucket, prefix, err := config.ParseS3URL(fs.Arg(0))
		if err != nil {
			return fatalf("%v", err)
		}
		fmt.Println()
		fmt.Printf("bucket s3://%s/%s:\n", bucket, prefix)
		cfg := config.Default()
		cfg.Bucket, cfg.Prefix = bucket, prefix
		cfg.Endpoint, cfg.Region, cfg.PathStyle = *endpoint, *region, *pathStyle
		cfg.Mountpoint = "/tmp"
		if err := cfg.Validate(); err != nil {
			return fatalf("%v", err)
		}
		// Say what is actually being contacted. A wrong endpoint is the most
		// common failure with a non-AWS provider, and it is invisible unless
		// the value in use is printed back.
		target := "AWS S3 (no endpoint set)"
		if cfg.Endpoint != "" {
			target = cfg.Endpoint
		}
		fmt.Printf("  via   %s  region=%s  path-style=%v\n", target, cfg.Region, cfg.PathStyle)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		client, err := s3io.New(ctx, cfg)
		check("client created", err, "")
		if err == nil {
			accessErr := client.CheckAccess(ctx)
			hint := ""
			if accessErr != nil {
				hint = diagnoseAccess(ctx, cfg)
			}
			check("bucket readable", accessErr, hint)
		}
	}

	fmt.Println()
	if failures == 0 {
		fmt.Println("all checks passed")
		return 0
	}
	fmt.Printf("%d check(s) failed\n", failures)
	return 1
}

// diagnoseAccess turns a failed bucket check into advice.
//
// Rather than guess, it retries with the other addressing style: most non-AWS
// providers do not resolve virtual-host names like bucket.example.com, and the
// resulting DNS timeout looks nothing like "you need --path-style". If the
// retry succeeds, that is the answer, and it is a fact rather than a hunch.
func diagnoseAccess(ctx context.Context, cfg *config.Config) string {
	if cfg.Endpoint != "" && !cfg.PathStyle {
		probe := *cfg
		probe.PathStyle = true
		pctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if client, err := s3io.New(pctx, &probe); err == nil {
			if client.CheckAccess(pctx) == nil {
				return "the bucket IS reachable with path-style addressing — " +
					"add --path-style to the mount options (most non-AWS providers need it)"
			}
		}
	}
	if cfg.Endpoint == "" {
		return "check the bucket name, the region, and the credentials"
	}
	return "check that the bucket exists at " + cfg.Endpoint + ", that the region is " +
		"the one it lives in, and that the credentials are for that provider"
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
