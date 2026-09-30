package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"strings"
	"time"

	"futrx.local/catalog/applications/s3disk/backend/container/internal/ctl"
	"futrx.local/catalog/applications/s3disk/backend/container/internal/humanize"
	"futrx.local/catalog/applications/s3disk/backend/container/internal/s3fs"
)

// ask makes one request to a running mount's control socket. The context lives
// exactly as long as the call: Get reads the whole reply before returning, so
// there is nothing left to cancel afterwards.
func ask(entry ctl.Entry, endpoint string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return ctl.NewClient(entry.Socket).Get(ctx, endpoint)
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
	body, err := ask(entry, "/status", 30*time.Second)
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
		humanize.Bytes(st.Cache.Bytes), humanize.Bytes(st.Cache.MaxBytes), st.Cache.Entries, st.Cache.Dirty)
	fmt.Printf("             %d hits, %d misses, %d evictions, %d uploads\n",
		st.Cache.Hits, st.Cache.Misses, st.Cache.Evictions, st.Cache.Uploads)
	fmt.Printf("s3 requests  %d HEAD  %d LIST  %d GET  %d PUT  %d COPY  %d DELETE  %d errors\n",
		st.S3.Heads, st.S3.Lists, st.S3.Gets, st.S3.Puts, st.S3.Copies, st.S3.Deletes, st.S3.Errors)
	fmt.Printf("transferred  %s down, %s up\n", humanize.Bytes(st.S3.BytesDown), humanize.Bytes(st.S3.BytesUp))
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
	body, err := ask(entry, "/sync", 60*time.Minute)
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
	if _, err := ask(entry, "/refresh", 5*time.Minute); err != nil {
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
