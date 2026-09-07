package main

import (
	"strings"
	"testing"
)

func TestOutputIsBounded(t *testing.T) {
	var out limitedOutput
	data := []byte(strings.Repeat("x", 100000))
	n, err := out.Write(data)
	if err != nil || n != len(data) || len(out.data) != 64*1024 {
		t.Fatal("output limit not enforced")
	}
}

// pushBackend is a backend whose container commands are recorded rather than
// run, with `mountpoint` and `mkdir` succeeding so a test only has to say what
// the interesting command does.
