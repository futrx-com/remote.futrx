package rpc

import (
	"errors"
	"fmt"
	"net/rpc"
	"sync"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

type BindAgentTurnsArgs struct{ BrokerID uint32 }
type BindAgentTurnsReply struct{ Error string }
type AgentStartArgs struct{ Request applications.AgentTurnRequest }
type AgentReadArgs struct{ Query applications.AgentTurnQuery }
type AgentForgetArgs struct{ RequestID string }
type AgentTurnReply struct {
	Turn        applications.AgentTurn
	Error, Code string
}

type runtimeAgentTurns struct {
	mu   sync.RWMutex
	impl applications.AgentTurns
}

func (r *runtimeAgentTurns) capability() (applications.AgentTurns, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.impl == nil {
		return nil, applications.ErrAgentUnavailable
	}
	return r.impl, nil
}
func (r *runtimeAgentTurns) Start(in applications.AgentTurnRequest) (applications.AgentTurn, error) {
	i, err := r.capability()
	if err != nil {
		return applications.AgentTurn{}, err
	}
	return i.Start(in)
}
func (r *runtimeAgentTurns) Read(in applications.AgentTurnQuery) (applications.AgentTurn, error) {
	i, err := r.capability()
	if err != nil {
		return applications.AgentTurn{}, err
	}
	return i.Read(in)
}
func (r *runtimeAgentTurns) Forget(id string) error {
	i, err := r.capability()
	if err != nil {
		return err
	}
	return i.Forget(id)
}
func (c *Client) BindAgentTurns(turns applications.AgentTurns) error {
	if turns == nil || c.broker == nil {
		return applications.ErrAgentUnavailable
	}
	id := c.broker.NextId()
	go c.broker.AcceptAndServe(id, &agentTurnsServer{impl: turns})
	var reply BindAgentTurnsReply
	if err := c.client.Call("Plugin.BindAgentTurns", BindAgentTurnsArgs{BrokerID: id}, &reply); err != nil {
		return err
	}
	if reply.Error != "" {
		return errors.New(reply.Error)
	}
	return nil
}
func (s *server) BindAgentTurns(args BindAgentTurnsArgs, reply *BindAgentTurnsReply) error {
	if s.turns == nil || s.broker == nil {
		reply.Error = "backend did not request application agent runtime"
		return nil
	}
	conn, err := s.broker.Dial(args.BrokerID)
	if err != nil {
		reply.Error = err.Error()
		return nil
	}
	s.turns.mu.Lock()
	defer s.turns.mu.Unlock()
	if s.turns.impl != nil {
		_ = conn.Close()
		reply.Error = "agent runtime already bound"
		return nil
	}
	s.turns.impl = &agentTurnsClient{client: rpc.NewClient(conn)}
	return nil
}

type agentTurnsServer struct{ impl applications.AgentTurns }

func (s *agentTurnsServer) Start(args AgentStartArgs, reply *AgentTurnReply) error {
	turn, err := recovered(func() (applications.AgentTurn, error) { return s.impl.Start(args.Request) })
	reply.Turn = turn
	setAgentError(reply, err)
	return nil
}
func (s *agentTurnsServer) Read(args AgentReadArgs, reply *AgentTurnReply) error {
	turn, err := recovered(func() (applications.AgentTurn, error) { return s.impl.Read(args.Query) })
	reply.Turn = turn
	setAgentError(reply, err)
	return nil
}
func (s *agentTurnsServer) Forget(args AgentForgetArgs, reply *AgentTurnReply) error {
	_, err := recovered(func() (struct{}, error) { return struct{}{}, s.impl.Forget(args.RequestID) })
	setAgentError(reply, err)
	return nil
}

var agentErrors = map[string]error{
	"access": applications.ErrAgentAccess, "busy": applications.ErrAgentBusy,
	"not-found": applications.ErrAgentTurnNotFound, "changed": applications.ErrAgentRequestChanged,
	"unavailable": applications.ErrAgentUnavailable,
}

func setAgentError(reply *AgentTurnReply, err error) {
	if err == nil {
		return
	}
	reply.Error = err.Error()
	for code, target := range agentErrors {
		if errors.Is(err, target) {
			reply.Code = code
			break
		}
	}
}

type agentTurnsClient struct{ client *rpc.Client }

func (c *agentTurnsClient) call(method string, args any) (applications.AgentTurn, error) {
	var reply AgentTurnReply
	if err := c.client.Call("Plugin."+method, args, &reply); err != nil {
		return applications.AgentTurn{}, err
	}
	if reply.Error != "" {
		if target := agentErrors[reply.Code]; target != nil {
			return applications.AgentTurn{}, fmt.Errorf("%w: %s", target, reply.Error)
		}
		return applications.AgentTurn{}, errors.New(reply.Error)
	}
	return reply.Turn, nil
}
func (c *agentTurnsClient) Start(in applications.AgentTurnRequest) (applications.AgentTurn, error) {
	return c.call("Start", AgentStartArgs{in})
}
func (c *agentTurnsClient) Read(in applications.AgentTurnQuery) (applications.AgentTurn, error) {
	return c.call("Read", AgentReadArgs{in})
}
func (c *agentTurnsClient) Forget(id string) error {
	_, err := c.call("Forget", AgentForgetArgs{id})
	return err
}
