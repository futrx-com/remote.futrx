package rpc

import (
	"errors"
	"net"
	"net/rpc"
	"testing"

	api "github.com/futrx-com/remote.futrx.com/pkg/applications"
)

type agentTurnsStub struct {
	panicStart bool
	forgot     string
}

func (s *agentTurnsStub) Start(api.AgentTurnRequest) (api.AgentTurn, error) {
	if s.panicStart {
		panic("provider panic")
	}
	return api.AgentTurn{Status: "running"}, nil
}
func (s *agentTurnsStub) Read(api.AgentTurnQuery) (api.AgentTurn, error) {
	return api.AgentTurn{}, api.ErrAgentTurnNotFound
}
func (s *agentTurnsStub) Forget(id string) error { s.forgot = id; return api.ErrAgentBusy }
func TestAgentCallbackPreservesErrorsAndRecoversPanics(t *testing.T) {
	stub := &agentTurnsStub{}
	server := rpc.NewServer()
	if err := server.RegisterName("Plugin", &agentTurnsServer{impl: stub}); err != nil {
		t.Fatal(err)
	}
	host, child := net.Pipe()
	defer host.Close()
	go server.ServeConn(host)
	transport := rpc.NewClient(child)
	defer transport.Close()
	client := &agentTurnsClient{client: transport}
	if turn, err := client.Start(api.AgentTurnRequest{}); err != nil || turn.Status != "running" {
		t.Fatalf("start: %+v %v", turn, err)
	}
	if _, err := client.Read(api.AgentTurnQuery{}); !errors.Is(err, api.ErrAgentTurnNotFound) {
		t.Fatalf("typed error: %v", err)
	}
	if err := client.Forget("job"); !errors.Is(err, api.ErrAgentBusy) || stub.forgot != "job" {
		t.Fatalf("forget: %v", err)
	}
	stub.panicStart = true
	if _, err := client.Start(api.AgentTurnRequest{}); err == nil {
		t.Fatal("panic disappeared across callback")
	}
}
