package rpc

import (
	"context"
	"fmt"
	"net"
	"net/rpc"
	"sync/atomic"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

// Client is the host's handle on a running backend process. It satisfies
// applications.Backend, so the host calls a backend exactly as a backend author
// implements one.
type Client struct {
	client        *rpc.Client
	nextRequestID atomic.Uint64
	broker        interface {
		NextId() uint32
		AcceptAndServe(uint32, any)
	}
}

type responseStreamBroker interface {
	Accept(uint32) (net.Conn, error)
}

var _ applications.Backend = (*Client)(nil)
var _ applications.EventSubscriber = (*Client)(nil)

func (c *Client) Describe() (applications.Descriptor, error) {
	var reply DescribeReply
	if err := c.client.Call("Plugin.Describe", DescribeArgs{}, &reply); err != nil {
		return applications.Descriptor{}, fmt.Errorf("describe: %w", err)
	}
	if reply.Error != "" {
		return applications.Descriptor{}, fmt.Errorf("describe: %s", reply.Error)
	}
	return reply.Descriptor, nil
}

func (c *Client) Init(instance applications.Instance) error {
	var reply InitReply
	if err := c.client.Call("Plugin.Init", InitArgs{Instance: instance}, &reply); err != nil {
		return fmt.Errorf("init: %w", err)
	}
	if reply.Error != "" {
		return fmt.Errorf("init: %s", reply.Error)
	}
	return nil
}

func (c *Client) Handle(request applications.Request) (applications.Response, error) {
	return c.HandleContext(context.Background(), request)
}

// HandleContext forwards one request and signals its cancellation without
// closing the shared backend connection. The ordinary Backend interface stays
// source-compatible; Remote's host detects this richer transport method.
func (c *Client) HandleContext(ctx context.Context, request applications.Request) (applications.Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	args := HandleArgs{RequestID: c.nextRequestID.Add(1), Request: request}
	streamBroker, hasStreamBroker := c.broker.(responseStreamBroker)
	if hasStreamBroker {
		args.StreamBrokerID = c.broker.NextId()
		args.StreamBroker = true
	}
	reply := new(HandleReply)
	call := c.client.Go("Plugin.Handle", args, reply, make(chan *rpc.Call, 1))
	select {
	case completed := <-call.Done:
		if completed.Error != nil {
			return applications.Response{}, fmt.Errorf("handle: %w", completed.Error)
		}
		return c.handleResponse(args, *reply, streamBroker, hasStreamBroker)
	case <-ctx.Done():
		// net/rpc supports concurrent calls on one connection. Go sends the
		// cancellation request before returning, while the abandoned Handle
		// reply is drained in the background so a late stream can still be
		// accepted and closed.
		c.client.Go("Plugin.Cancel", CancelArgs{
			RequestID:        args.RequestID,
			DeadlineExceeded: ctx.Err() == context.DeadlineExceeded,
		}, new(CancelReply), nil)
		go c.discardHandleResponse(call, args, reply, streamBroker, hasStreamBroker)
		return applications.Response{}, fmt.Errorf("handle: %w", ctx.Err())
	}
}

func (c *Client) handleResponse(
	args HandleArgs,
	reply HandleReply,
	streamBroker responseStreamBroker,
	hasStreamBroker bool,
) (applications.Response, error) {
	if reply.Error != "" {
		return applications.Response{}, fmt.Errorf("handle: %s", reply.Error)
	}
	if reply.Stream != nil {
		if !hasStreamBroker || !args.StreamBroker {
			return applications.Response{}, fmt.Errorf("handle: backend returned a stream without a broker")
		}
		connection, err := streamBroker.Accept(args.StreamBrokerID)
		if err != nil {
			return applications.Response{}, fmt.Errorf("handle: accept response stream: %w", err)
		}
		stream := newRemoteStream(rpc.NewClient(connection), reply.Stream.Size)
		response := applications.Stream(stream, reply.Stream.Size, reply.Stream.ModTime, reply.Response.Headers)
		response.Status = reply.Response.Status
		return response, nil
	}
	return reply.Response, nil
}

func (c *Client) discardHandleResponse(
	call *rpc.Call,
	args HandleArgs,
	reply *HandleReply,
	streamBroker responseStreamBroker,
	hasStreamBroker bool,
) {
	completed := <-call.Done
	if completed.Error != nil {
		return
	}
	response, err := c.handleResponse(args, *reply, streamBroker, hasStreamBroker)
	if err != nil {
		return
	}
	if content, _, _, ok := response.ResponseStream(); ok && content != nil {
		_ = content.Close()
	}
}

// BindEvents opens the core-owned callback connection used by Runtime.Events.
// It is transport plumbing called by the host, not an application backend
// capability.
func (c *Client) BindEvents(emitter applications.EventEmitter) error {
	if emitter == nil {
		return fmt.Errorf("bind events: emitter is nil")
	}
	if c.broker == nil {
		return fmt.Errorf("bind events: callback broker is unavailable")
	}
	id := c.broker.NextId()
	go c.broker.AcceptAndServe(id, &eventEmitterServer{impl: emitter})

	var reply BindEventsReply
	if err := c.client.Call("Plugin.BindEvents", BindEventsArgs{BrokerID: id}, &reply); err != nil {
		return fmt.Errorf("bind events: %w", err)
	}
	if reply.Error != "" {
		return fmt.Errorf("bind events: %s", reply.Error)
	}
	return nil
}

// OnEvent delivers one event over the backend's primary RPC connection.
func (c *Client) OnEvent(event applications.Event) error {
	var reply OnEventReply
	if err := c.client.Call("Plugin.OnEvent", OnEventArgs{Event: event}, &reply); err != nil {
		return fmt.Errorf("on event: %w", err)
	}
	if reply.Error != "" {
		return fmt.Errorf("on event: %s", reply.Error)
	}
	return nil
}

// eventEmitterClient is the application side of the core-owned callback.
type eventEmitterClient struct {
	client *rpc.Client
}

var _ applications.EventEmitter = (*eventEmitterClient)(nil)

func (c *eventEmitterClient) Emit(publication applications.Publication) error {
	if len(publication.Payload) > applications.MaxEventPayloadBytes {
		return fmt.Errorf(
			"emit event: payload exceeds %d bytes",
			applications.MaxEventPayloadBytes,
		)
	}
	var reply EmitReply
	if err := c.client.Call("Plugin.Emit", EmitArgs{Publication: publication}, &reply); err != nil {
		return fmt.Errorf("emit event: %w", err)
	}
	if reply.Error != "" {
		return fmt.Errorf("emit event: %s", reply.Error)
	}
	return nil
}
