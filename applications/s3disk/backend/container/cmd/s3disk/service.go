package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

// runService is the stable command in application.json. Remote maps declared
// install values to base64 environment variables so credentials and mount
// options cannot alter the systemd environment-file syntax.
func runService() int {
	args, err := serviceMountArgs()
	if err != nil {
		return fatalf("service configuration: %v", err)
	}
	return runMount(args)
}

func serviceMountArgs() ([]string, error) {
	values := make(map[string]string)
	for _, key := range []string{
		"S3DISK_BUCKET", "S3DISK_MOUNTPOINT", "AWS_ACCESS_KEY_ID",
		"AWS_SECRET_ACCESS_KEY", "AWS_REGION", "S3DISK_ENDPOINT", "S3DISK_MOUNT_ARGS",
	} {
		decoded, err := base64.StdEncoding.DecodeString(os.Getenv(key + "_B64"))
		if err != nil {
			return nil, fmt.Errorf("%s_B64 is not valid base64", key)
		}
		values[key] = string(decoded)
	}
	for _, key := range []string{"S3DISK_BUCKET", "S3DISK_MOUNTPOINT", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"} {
		if values[key] == "" {
			return nil, fmt.Errorf("%s is required", key)
		}
	}

	extra, err := splitMountArgs(values["S3DISK_MOUNT_ARGS"])
	if err != nil {
		return nil, fmt.Errorf("S3DISK_MOUNT_ARGS: %w", err)
	}
	for _, arg := range extra {
		if arg == "--" || strings.HasPrefix(arg, "--foreground") {
			return nil, fmt.Errorf("S3DISK_MOUNT_ARGS cannot override the service foreground mode")
		}
	}

	for _, key := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_REGION"} {
		if err := os.Setenv(key, values[key]); err != nil {
			return nil, fmt.Errorf("set %s: %w", key, err)
		}
	}
	args := []string{values["S3DISK_BUCKET"], values["S3DISK_MOUNTPOINT"]}
	if endpoint := values["S3DISK_ENDPOINT"]; endpoint != "" {
		args = append(args, "--endpoint", endpoint)
	}
	if !hasExclusiveOption(extra) {
		args = append(args, "--exclusive")
	}
	args = append(args, extra...)
	args = append(args, "--foreground")
	return args, nil
}

func hasExclusiveOption(args []string) bool {
	for _, arg := range args {
		if arg == "--exclusive" || arg == "--no-exclusive" ||
			strings.HasPrefix(arg, "--exclusive=") || strings.HasPrefix(arg, "--no-exclusive=") {
			return true
		}
	}
	return false
}

// splitMountArgs accepts the quoting and backslash escaping users expect in a
// flag string, without invoking a shell or interpreting substitutions.
func splitMountArgs(input string) ([]string, error) {
	var result []string
	var current strings.Builder
	var quote rune
	escaped := false
	started := false
	for _, r := range input {
		if escaped {
			current.WriteRune(r)
			escaped = false
			started = true
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			started = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
			started = true
		case ' ', '\t', '\n', '\r':
			if started {
				result = append(result, current.String())
				current.Reset()
				started = false
			}
		default:
			current.WriteRune(r)
			started = true
		}
	}
	if escaped || quote != 0 {
		return nil, fmt.Errorf("unterminated escape or quote")
	}
	if started {
		result = append(result, current.String())
	}
	return result, nil
}
