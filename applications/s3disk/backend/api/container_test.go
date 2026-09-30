package api

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
