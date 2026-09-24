package rpc

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/rpc"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

type observedReadSeekCloser struct {
	*bytes.Reader
	mu          sync.Mutex
	maxReadSize int
	closed      chan struct{}
	closeOnce   sync.Once
}

func (r *observedReadSeekCloser) Read(buffer []byte) (int, error) {
	r.mu.Lock()
	if len(buffer) > r.maxReadSize {
		r.maxReadSize = len(buffer)
	}
	r.mu.Unlock()
	return r.Reader.Read(buffer)
}

func (r *observedReadSeekCloser) Close() error {
	r.closeOnce.Do(func() { close(r.closed) })
	return nil
}

func TestStreamReaderServerUsesAbsoluteBoundedReads(t *testing.T) {
	content := &observedReadSeekCloser{
		Reader: bytes.NewReader([]byte("0123456789")),
		closed: make(chan struct{}),
	}
	server := &streamReaderServer{content: content}

	var reply StreamReadReply
	if err := server.Read(StreamReadArgs{Offset: 4, Length: 3}, &reply); err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if string(reply.Data) != "456" || reply.Error != "" || reply.EOF {
		t.Fatalf("reply = %+v", reply)
	}

	reply = StreamReadReply{}
	if err := server.Read(StreamReadArgs{Offset: 0, Length: maxStreamReadBytes + 1}, &reply); err != nil {
		t.Fatalf("oversized Read() RPC error: %v", err)
	}
	if !strings.Contains(reply.Error, "between 1") {
		t.Fatalf("oversized read error = %q", reply.Error)
	}
	content.mu.Lock()
	maxReadSize := content.maxReadSize
	content.mu.Unlock()
	if maxReadSize != 3 {
		t.Fatalf("content saw a %d-byte read after the oversized request", maxReadSize)
	}
}

func TestRemoteStreamReadsSeeksAndCloses(t *testing.T) {
	data := bytes.Repeat([]byte("0123456789abcdef"), (maxStreamReadBytes/16)+2)
	content := &observedReadSeekCloser{
		Reader: bytes.NewReader(data),
		closed: make(chan struct{}),
	}
	serverConnection, clientConnection := net.Pipe()
	server := rpc.NewServer()
	reader := &streamReaderServer{content: content}
	if err := server.RegisterName("Stream", reader); err != nil {
		t.Fatalf("register stream: %v", err)
	}
	go server.ServeConn(&closeContentConn{Conn: serverConnection, closeContent: reader.close})
	stream := newRemoteStream(rpc.NewClient(clientConnection), int64(len(data)))

	first := make([]byte, 37)
	if _, err := io.ReadFull(stream, first); err != nil {
		t.Fatalf("read first bytes: %v", err)
	}
	if !bytes.Equal(first, data[:len(first)]) {
		t.Fatalf("first bytes = %q", first)
	}
	if position, err := stream.Seek(-23, io.SeekEnd); err != nil || position != int64(len(data)-23) {
		t.Fatalf("SeekEnd() = %d, %v", position, err)
	}
	last, err := io.ReadAll(stream)
	if err != nil || !bytes.Equal(last, data[len(data)-23:]) {
		t.Fatalf("last bytes = %q, %v", last, err)
	}
	content.mu.Lock()
	maxReadSize := content.maxReadSize
	content.mu.Unlock()
	if maxReadSize > maxStreamReadBytes {
		t.Fatalf("backend read allocation = %d, max %d", maxReadSize, maxStreamReadBytes)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	select {
	case <-content.closed:
	case <-time.After(time.Second):
		t.Fatal("closing the host stream did not close backend content")
	}
	if _, err := stream.Read(first); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("read after close error = %v, want net.ErrClosed", err)
	}
}

type blockingReadSeekCloser struct {
	closed    chan struct{}
	closeOnce sync.Once
	started   chan struct{}
	startOnce sync.Once
}

func (r *blockingReadSeekCloser) Read([]byte) (int, error) {
	r.startOnce.Do(func() { close(r.started) })
	<-r.closed
	return 0, net.ErrClosed
}

func (*blockingReadSeekCloser) Seek(offset int64, _ int) (int64, error) { return offset, nil }

func (r *blockingReadSeekCloser) Close() error {
	r.closeOnce.Do(func() { close(r.closed) })
	return nil
}

func TestClosingBrokerConnectionInterruptsBackendRead(t *testing.T) {
	content := &blockingReadSeekCloser{closed: make(chan struct{}), started: make(chan struct{})}
	reader := &streamReaderServer{content: content}
	serverConnection, clientConnection := net.Pipe()
	connection := &closeContentConn{Conn: serverConnection, closeContent: reader.close}
	server := rpc.NewServer()
	if err := server.RegisterName("Stream", reader); err != nil {
		t.Fatalf("register stream: %v", err)
	}
	served := make(chan struct{})
	go func() {
		server.ServeConn(connection)
		close(served)
	}()
	client := rpc.NewClient(clientConnection)
	called := make(chan error, 1)
	go func() {
		var reply StreamReadReply
		called <- client.Call("Stream.Read", StreamReadArgs{Offset: 0, Length: 1}, &reply)
	}()

	select {
	case <-content.started:
	case <-time.After(time.Second):
		t.Fatal("backend read did not start")
	}
	if err := client.Close(); err != nil {
		t.Fatalf("close client: %v", err)
	}
	select {
	case <-content.closed:
	case <-time.After(time.Second):
		t.Fatal("connection close did not interrupt backend content")
	}
	select {
	case <-served:
	case <-time.After(time.Second):
		t.Fatal("RPC server remained blocked after disconnect")
	}
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("host RPC call remained blocked after disconnect")
	}
}

type fixedResponseBackend struct {
	response applications.Response
	err      error
}

func (*fixedResponseBackend) Describe() (applications.Descriptor, error) {
	return applications.Descriptor{APIVersion: applications.APIVersion}, nil
}

func (*fixedResponseBackend) Init(applications.Instance) error { return nil }

func (b *fixedResponseBackend) Handle(applications.Request) (applications.Response, error) {
	return b.response, b.err
}

func TestServerRejectsInvalidStreamsAndClosesTheirContent(t *testing.T) {
	tests := []struct {
		name   string
		change func(*applications.Response)
		want   string
	}{
		{
			name: "negative size",
			change: func(response *applications.Response) {
				content, _, modTime, _ := response.ResponseStream()
				*response = applications.Stream(content, -1, modTime, nil)
			},
			want: "size is negative",
		},
		{
			name: "buffer and stream",
			change: func(response *applications.Response) {
				response.Body = []byte("also buffered")
			},
			want: "also contains a buffered body",
		},
		{
			name: "custom status",
			change: func(response *applications.Response) {
				response.Status = 201
			},
			want: "status must be 0 or 200",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			content := &observedReadSeekCloser{
				Reader: bytes.NewReader([]byte("content")),
				closed: make(chan struct{}),
			}
			response := applications.Stream(content, 7, time.Time{}, nil)
			tc.change(&response)
			transport := &server{impl: &fixedResponseBackend{response: response}}
			var reply HandleReply
			if err := transport.Handle(HandleArgs{StreamBroker: true}, &reply); err != nil {
				t.Fatalf("Handle() RPC error: %v", err)
			}
			if !strings.Contains(reply.Error, tc.want) {
				t.Fatalf("reply error = %q, want %q", reply.Error, tc.want)
			}
			select {
			case <-content.closed:
			default:
				t.Fatal("invalid stream content was not closed")
			}
		})
	}
}

func TestServerClosesStreamWhenBackendAlsoReturnsAnError(t *testing.T) {
	content := &observedReadSeekCloser{
		Reader: bytes.NewReader([]byte("content")),
		closed: make(chan struct{}),
	}
	response := applications.Stream(content, 7, time.Time{}, nil)
	transport := &server{impl: &fixedResponseBackend{response: response, err: errors.New("failed")}}
	var reply HandleReply
	if err := transport.Handle(HandleArgs{StreamBroker: true}, &reply); err != nil {
		t.Fatalf("Handle() RPC error: %v", err)
	}
	if reply.Error != "failed" {
		t.Fatalf("reply error = %q", reply.Error)
	}
	select {
	case <-content.closed:
	default:
		t.Fatal("errored stream content was not closed")
	}
}
