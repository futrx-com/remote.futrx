package rpc

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/rpc"
	"sync"
	"sync/atomic"

	goplugin "github.com/hashicorp/go-plugin"
)

// maxStreamReadBytes bounds allocation on both sides of one brokered read.
// The host prefetches at this size so ServeContent's smaller copy buffer does
// not turn a large download into tens of thousands of primary RPC calls.
const maxStreamReadBytes = 256 << 10

type streamReaderServer struct {
	mu        sync.Mutex
	content   io.ReadSeekCloser
	closed    atomic.Bool
	closeOnce sync.Once
	closeErr  error
}

func (s *streamReaderServer) Read(args StreamReadArgs, reply *StreamReadReply) (rpcError error) {
	defer func() {
		if recovery := recover(); recovery != nil {
			reply.Data = nil
			reply.EOF = false
			reply.Error = fmt.Sprintf("response stream panicked: %v", recovery)
		}
	}()
	if args.Offset < 0 {
		reply.Error = "stream read offset is negative"
		return nil
	}
	if args.Length <= 0 || args.Length > maxStreamReadBytes {
		reply.Error = fmt.Sprintf("stream read length must be between 1 and %d bytes", maxStreamReadBytes)
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed.Load() {
		reply.Error = "stream is closed"
		return nil
	}
	position, err := s.content.Seek(args.Offset, io.SeekStart)
	if err != nil {
		reply.Error = err.Error()
		return nil
	}
	if position != args.Offset {
		reply.Error = fmt.Sprintf("stream seek reached offset %d instead of %d", position, args.Offset)
		return nil
	}
	buffer := make([]byte, args.Length)
	n, err := s.content.Read(buffer)
	reply.Data = buffer[:n]
	if errors.Is(err, io.EOF) {
		reply.EOF = true
	} else if err != nil {
		reply.Error = err.Error()
	}
	return nil
}

func (s *streamReaderServer) close() error {
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		s.closeErr = closeWithoutPanic(s.content)
	})
	return s.closeErr
}

func closeWithoutPanic(content io.Closer) (err error) {
	defer func() {
		if recovery := recover(); recovery != nil {
			err = fmt.Errorf("close response stream panicked: %v", recovery)
		}
	}()
	return content.Close()
}

// closeContentConn closes the application-owned reader as soon as the host
// side disappears. net/rpc otherwise waits for active method goroutines before
// closing its codec, which can deadlock if one of those reads needs Close to
// unblock it.
type closeContentConn struct {
	net.Conn
	closeContent func() error
	once         sync.Once
}

func (c *closeContentConn) Read(buffer []byte) (int, error) {
	n, err := c.Conn.Read(buffer)
	if err != nil {
		c.once.Do(func() { _ = c.closeContent() })
	}
	return n, err
}

func (c *closeContentConn) Close() error {
	c.once.Do(func() { _ = c.closeContent() })
	return c.Conn.Close()
}

func serveStream(broker *goplugin.MuxBroker, id uint32, content io.ReadSeekCloser) {
	reader := &streamReaderServer{content: content}
	connection, err := broker.Dial(id)
	if err != nil {
		_ = reader.close()
		return
	}
	connection = &closeContentConn{Conn: connection, closeContent: reader.close}
	server := rpc.NewServer()
	if err := server.RegisterName("Stream", reader); err != nil {
		_ = connection.Close()
		return
	}
	server.ServeConn(connection)
	_ = connection.Close()
}

// remoteStream makes the brokered random-access protocol look like the
// io.ReadSeeker expected by http.ServeContent. It knows the declared length,
// so SeekEnd never requires another application call.
type remoteStream struct {
	client *rpc.Client
	size   int64

	mu         sync.Mutex
	position   int64
	buffer     []byte
	pendingErr error
	closed     atomic.Bool
	closeOnce  sync.Once
}

func newRemoteStream(client *rpc.Client, size int64) *remoteStream {
	return &remoteStream{client: client, size: size}
}

func (s *remoteStream) Read(destination []byte) (int, error) {
	if len(destination) == 0 {
		return 0, nil
	}
	if s.closed.Load() {
		return 0, net.ErrClosed
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed.Load() {
		return 0, net.ErrClosed
	}
	if len(s.buffer) == 0 && s.pendingErr != nil {
		err := s.pendingErr
		s.pendingErr = nil
		return 0, err
	}
	if len(s.buffer) == 0 {
		if s.position >= s.size {
			return 0, io.EOF
		}
		length := int64(maxStreamReadBytes)
		if remaining := s.size - s.position; remaining < length {
			length = remaining
		}
		var reply StreamReadReply
		if err := s.client.Call("Stream.Read", StreamReadArgs{
			Offset: s.position,
			Length: int(length),
		}, &reply); err != nil {
			return 0, fmt.Errorf("read response stream: %w", err)
		}
		if len(reply.Data) > int(length) || int64(len(reply.Data)) > s.size-s.position {
			return 0, fmt.Errorf("read response stream: backend returned data beyond the requested range")
		}
		s.buffer = reply.Data
		switch {
		case reply.Error != "":
			s.pendingErr = errors.New(reply.Error)
		case reply.EOF:
			s.pendingErr = io.EOF
		case len(reply.Data) == 0:
			s.pendingErr = io.ErrNoProgress
		}
		if len(s.buffer) == 0 {
			err := s.pendingErr
			s.pendingErr = nil
			return 0, err
		}
	}

	n := copy(destination, s.buffer)
	s.buffer = s.buffer[n:]
	s.position += int64(n)
	if len(s.buffer) == 0 && s.pendingErr != nil {
		err := s.pendingErr
		s.pendingErr = nil
		return n, err
	}
	return n, nil
}

func (s *remoteStream) Seek(offset int64, whence int) (int64, error) {
	if s.closed.Load() {
		return 0, net.ErrClosed
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed.Load() {
		return 0, net.ErrClosed
	}

	var base int64
	switch whence {
	case io.SeekStart:
		base = 0
	case io.SeekCurrent:
		base = s.position
	case io.SeekEnd:
		base = s.size
	default:
		return 0, fmt.Errorf("seek response stream: invalid whence %d", whence)
	}
	position, ok := addInt64(base, offset)
	if !ok || position < 0 {
		return 0, fmt.Errorf("seek response stream: invalid offset %d", offset)
	}
	s.position = position
	s.buffer = nil
	s.pendingErr = nil
	return position, nil
}

func (s *remoteStream) Close() error {
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		_ = s.client.Close()
	})
	return nil
}

func addInt64(left, right int64) (int64, bool) {
	result := left + right
	if (right > 0 && result < left) || (right < 0 && result > left) {
		return 0, false
	}
	return result, true
}
