package rpc

import (
	"time"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

// net/rpc requires exported argument and reply types, and carries an error
// only as a string. Each reply therefore has its own Error field, which keeps
// "the backend returned an error" distinct from "the call did not arrive".

type DescribeArgs struct{}

type DescribeReply struct {
	Descriptor applications.Descriptor
	Error      string
}

type InitArgs struct {
	Instance applications.Instance
}

type InitReply struct {
	Error string
}

type HandleArgs struct {
	RequestID      uint64
	Request        applications.Request
	StreamBrokerID uint32
	StreamBroker   bool
}

type HandleReply struct {
	Response applications.Response
	Stream   *StreamInfo
	Error    string
}

// CancelArgs identifies one Handle call whose host context has ended. Calls
// share the primary net/rpc connection, so cancellation must never close that
// connection or affect another request in the same backend process.
type CancelArgs struct {
	RequestID        uint64
	DeadlineExceeded bool
}

type CancelReply struct {
	Acknowledged bool
}

// StreamInfo describes content that travels over the brokered random-access
// connection identified by HandleArgs.StreamBrokerID.
type StreamInfo struct {
	Size    int64
	ModTime time.Time
}

type StreamReadArgs struct {
	Offset int64
	Length int
}

type StreamReadReply struct {
	Data  []byte
	EOF   bool
	Error string
}

// BindEventsArgs carries the broker stream backing the core-owned event
// runtime. The emitter itself cannot be encoded by net/rpc, so go-plugin's
// MuxBroker supplies a second RPC connection.
type BindEventsArgs struct {
	BrokerID uint32
}

type BindEventsReply struct {
	Error string
}

type EmitArgs struct {
	Publication applications.Publication
}

type EmitReply struct {
	Error string
}

type OnEventArgs struct {
	Event applications.Event
}

type OnEventReply struct {
	Error string
}
