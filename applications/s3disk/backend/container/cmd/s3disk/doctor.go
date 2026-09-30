package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"

	"futrx.local/catalog/applications/s3disk/backend/container/internal/config"
	"futrx.local/catalog/applications/s3disk/backend/container/internal/s3io"
)

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
