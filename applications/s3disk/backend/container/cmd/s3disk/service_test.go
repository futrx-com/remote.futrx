package main

import (
	"encoding/base64"
	"os"
	"reflect"
	"strings"
	"testing"
)

func setServiceInput(t *testing.T, key, value string) {
	t.Helper()
	t.Setenv(key+"_B64", base64.StdEncoding.EncodeToString([]byte(value)))
}

func TestServiceMountArgs(t *testing.T) {
	setServiceInput(t, "S3DISK_BUCKET", "s3://photos/archive")
	setServiceInput(t, "S3DISK_MOUNTPOINT", "/workspace/picture library")
	setServiceInput(t, "AWS_ACCESS_KEY_ID", "user\nname")
	setServiceInput(t, "AWS_SECRET_ACCESS_KEY", "secret with spaces")
	setServiceInput(t, "AWS_REGION", "eu-west-1")
	setServiceInput(t, "S3DISK_ENDPOINT", "https://s3.example.test:9000")
	setServiceInput(t, "S3DISK_MOUNT_ARGS", `--path-style --cache-dir "/workspace/cache dir"`)
	args, err := serviceMountArgs()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"s3://photos/archive", "/workspace/picture library",
		"--endpoint", "https://s3.example.test:9000", "--exclusive",
		"--path-style", "--cache-dir", "/workspace/cache dir", "--foreground",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
	if got := os.Getenv("AWS_SECRET_ACCESS_KEY"); got != "secret with spaces" {
		t.Fatalf("secret was not restored to the process environment")
	}
}

func TestServiceMountArgsRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name, args, want string
	}{
		{"unterminated quote", `--cache-dir "unfinished`, "unterminated"},
		{"foreground override", "--foreground=false", "foreground mode"},
		{"argument terminator", "--", "foreground mode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setServiceInput(t, "S3DISK_BUCKET", "s3://photos")
			setServiceInput(t, "S3DISK_MOUNTPOINT", "/workspace/s3")
			setServiceInput(t, "AWS_ACCESS_KEY_ID", "user")
			setServiceInput(t, "AWS_SECRET_ACCESS_KEY", "secret")
			setServiceInput(t, "AWS_REGION", "us-east-1")
			setServiceInput(t, "S3DISK_ENDPOINT", "")
			setServiceInput(t, "S3DISK_MOUNT_ARGS", tt.args)
			_, err := serviceMountArgs()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestServiceMountArgsDoesNotOverrideExplicitSharedMount(t *testing.T) {
	setServiceInput(t, "S3DISK_BUCKET", "s3://photos")
	setServiceInput(t, "S3DISK_MOUNTPOINT", "/workspace/s3")
	setServiceInput(t, "AWS_ACCESS_KEY_ID", "user")
	setServiceInput(t, "AWS_SECRET_ACCESS_KEY", "secret")
	setServiceInput(t, "AWS_REGION", "us-east-1")
	setServiceInput(t, "S3DISK_ENDPOINT", "")
	setServiceInput(t, "S3DISK_MOUNT_ARGS", "--no-exclusive")
	args, err := serviceMountArgs()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(args, " ") != "s3://photos /workspace/s3 --no-exclusive --foreground" {
		t.Fatalf("unexpected arguments: %#v", args)
	}
}
