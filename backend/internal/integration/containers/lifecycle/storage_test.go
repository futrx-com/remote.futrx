package lifecycle

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

func TestClientStorageSpaceReadsTheDefaultProfilePool(t *testing.T) {
	runner := &recordingRunner{available: true, responses: []runnerResponse{
		{out: `{"devices":{"root":{"path":"/","pool":"fast","type":"disk"}}}`},
		{out: `{"space":{"used":30,"total":100},"inodes":{"used":1,"total":2}}`},
	}}

	pool, free, total, err := NewClient(runner).StorageSpace(context.Background())
	if err != nil {
		t.Fatalf("StorageSpace() error = %v", err)
	}
	if pool != "fast" || free != 70 || total != 100 {
		t.Fatalf("StorageSpace() = %q, %d, %d; want fast, 70, 100", pool, free, total)
	}
	want := [][]string{
		{"query", "/1.0/profiles/default"},
		{"query", "/1.0/storage-pools/fast/resources"},
	}
	if !slices.EqualFunc(runner.calls, want, slices.Equal[[]string]) {
		t.Fatalf("calls = %q, want %q", runner.calls, want)
	}
}

func TestClientStorageSpaceRejectsUnmeasurablePools(t *testing.T) {
	tests := []struct {
		name      string
		responses []runnerResponse
		wantErr   string
	}{
		{
			name:      "no root pool",
			responses: []runnerResponse{{out: `{"devices":{}}`}},
			wantErr:   "default profile has no root disk pool",
		},
		{
			name: "no capacity",
			responses: []runnerResponse{
				{out: `{"devices":{"root":{"pool":"default"}}}`},
				{out: `{"space":{"used":0,"total":0}}`},
			},
			wantErr: "storage pool default reports no capacity",
		},
		{
			name:      "query fails",
			responses: []runnerResponse{{out: "Error: not authorized", err: errors.New("exit 1")}},
			wantErr:   "read default profile",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &recordingRunner{available: true, responses: test.responses}
			_, _, _, err := NewClient(runner).StorageSpace(context.Background())
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("StorageSpace() error = %v, want it to contain %q", err, test.wantErr)
			}
		})
	}
}

func TestClientStorageSpaceNeverUnderflowsAnOverfullPool(t *testing.T) {
	runner := &recordingRunner{available: true, responses: []runnerResponse{
		{out: `{"devices":{"root":{"pool":"default"}}}`},
		{out: `{"space":{"used":120,"total":100}}`},
	}}
	_, free, _, err := NewClient(runner).StorageSpace(context.Background())
	if err != nil || free != 0 {
		t.Fatalf("StorageSpace() free = %d, error = %v; want 0, nil", free, err)
	}
}

func TestClientImageSizeFollowsTheAlias(t *testing.T) {
	runner := &recordingRunner{available: true, responses: []runnerResponse{
		{out: `{"name":"remote-base","target":"abc123","type":"container"}`},
		{out: `{"fingerprint":"abc123","size":4294967296}`},
	}}

	size, err := NewClient(runner).ImageSize(context.Background(), "remote-base")
	if err != nil || size != 4<<30 {
		t.Fatalf("ImageSize() = %d, %v; want %d, nil", size, err, uint64(4<<30))
	}
	want := [][]string{
		{"query", "/1.0/images/aliases/remote-base"},
		{"query", "/1.0/images/abc123"},
	}
	if !slices.EqualFunc(runner.calls, want, slices.Equal[[]string]) {
		t.Fatalf("calls = %q, want %q", runner.calls, want)
	}
}

func TestClientInitReportsAFullPoolAsInsufficientStorage(t *testing.T) {
	out := "Creating project-1\r\nRetrieving image: Unpacking: 12%\r\n" +
		"Error: Failed creating instance from image: Unpack failed: tar: rootfs/var/cache: Cannot mkdir: No space left on device\n"
	runner := &recordingRunner{available: true, responses: []runnerResponse{{out: out, err: errors.New("exit status 1")}}}

	err := NewClient(runner).Init(context.Background(), "local:remote-base", "project-1")
	if !errors.Is(err, serviceproject.ErrInsufficientStorage) {
		t.Fatalf("Init() error = %v, want ErrInsufficientStorage", err)
	}
	if strings.Contains(err.Error(), "Retrieving image") {
		t.Fatalf("Init() error = %q, want no transfer progress", err)
	}
}

func TestClientInitKeepsTheCauseAndDropsTransferProgress(t *testing.T) {
	var out strings.Builder
	out.WriteString("Creating project-1\n")
	for i := range 500 {
		out.WriteString("Retrieving image: Unpacking image: " + strings.Repeat("#", i%40) + "\r")
	}
	out.WriteString("\nError: Failed instance creation: Image not found\n")
	runner := &recordingRunner{available: true, responses: []runnerResponse{{out: out.String(), err: errors.New("exit status 1")}}}

	err := NewClient(runner).Init(context.Background(), "local:remote-base", "project-1")
	if err == nil {
		t.Fatal("Init() error = nil, want failure")
	}
	message := err.Error()
	if strings.Contains(message, "Retrieving image") || strings.Contains(message, "Unpacking image") {
		t.Fatalf("Init() error keeps transfer progress: %q", message)
	}
	if !strings.Contains(message, "Error: Failed instance creation: Image not found") {
		t.Fatalf("Init() error = %q, want the LXD error", message)
	}
	if len(message) > initFailureOutputBytes+200 {
		t.Fatalf("Init() error is %d bytes, want it bounded", len(message))
	}
}
