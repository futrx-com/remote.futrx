package pluginrpc

import (
	"fmt"
	"net/rpc"

	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
)

// Client is the host's handle on a running plugin process. It satisfies
// appplugin.Backend, so the host calls a plugin exactly as a plugin author
// implements one.
type Client struct {
	client *rpc.Client
}

var _ appplugin.Backend = (*Client)(nil)

func (c *Client) Describe() (appplugin.Descriptor, error) {
	var reply DescribeReply
	if err := c.client.Call("Plugin.Describe", DescribeArgs{}, &reply); err != nil {
		return appplugin.Descriptor{}, fmt.Errorf("describe: %w", err)
	}
	if reply.Error != "" {
		return appplugin.Descriptor{}, fmt.Errorf("describe: %s", reply.Error)
	}
	return reply.Descriptor, nil
}

func (c *Client) Init(instance appplugin.Instance) error {
	var reply InitReply
	if err := c.client.Call("Plugin.Init", InitArgs{Instance: instance}, &reply); err != nil {
		return fmt.Errorf("init: %w", err)
	}
	if reply.Error != "" {
		return fmt.Errorf("init: %s", reply.Error)
	}
	return nil
}

func (c *Client) Handle(request appplugin.Request) (appplugin.Response, error) {
	var reply HandleReply
	if err := c.client.Call("Plugin.Handle", HandleArgs{Request: request}, &reply); err != nil {
		return appplugin.Response{}, fmt.Errorf("handle: %w", err)
	}
	if reply.Error != "" {
		return appplugin.Response{}, fmt.Errorf("handle: %s", reply.Error)
	}
	return reply.Response, nil
}
