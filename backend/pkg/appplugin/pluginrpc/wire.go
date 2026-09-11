package pluginrpc

import "github.com/futrx-com/remote.futrx.com/pkg/appplugin"

// net/rpc requires exported argument and reply types, and carries an error
// only as a string. Each reply therefore has its own Error field, which keeps
// "the plugin returned an error" distinct from "the call did not arrive".

type DescribeArgs struct{}

type DescribeReply struct {
	Descriptor appplugin.Descriptor
	Error      string
}

type InitArgs struct {
	Instance appplugin.Instance
}

type InitReply struct {
	Error string
}

type HandleArgs struct {
	Request appplugin.Request
}

type HandleReply struct {
	Response appplugin.Response
	Error    string
}
