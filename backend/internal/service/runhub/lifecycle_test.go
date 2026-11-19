package runhub

import (
	"context"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	"testing"
	"time"
)

type versionedStore struct {
	version        string
	versionRead    chan struct{}
	releaseVersion chan struct{}
}

func (s *versionedStore) HistoryVersion(context.Context, servicechat.ID) (string, error) {
	version := s.version
	if s.versionRead != nil {
		close(s.versionRead)
		<-s.releaseVersion
	}
	return version, nil
}
func (s *versionedStore) ReadEvents(context.Context, servicechat.ID) ([]servicechat.Event, error) {
	return []servicechat.Event{{Seq: 1, Type: "user", Text: s.version}}, nil
}
func (s *versionedStore) ReadEventsAfter(ctx context.Context, id servicechat.ID, _ int64) ([]servicechat.Event, error) {
	return s.ReadEvents(ctx, id)
}
func (s *versionedStore) AppendEvent(_ context.Context, _ servicechat.ID, event servicechat.Event) (servicechat.Event, error) {
	return event, nil
}

func TestSubscribeKeepsHistoryVersionAndReplayOnSameBranch(t *testing.T) {
	store := &versionedStore{version: "old", versionRead: make(chan struct{}), releaseVersion: make(chan struct{})}
	hub := New(store)
	subscribed := make(chan *Subscription, 1)
	go func() {
		sub, err := hub.Subscribe(context.Background(), "abcd")
		if err != nil {
			panic(err)
		}
		subscribed <- sub
	}()
	<-store.versionRead
	mutated := make(chan struct{})
	go func() {
		if err := hub.MutateIdle("abcd", func() error { store.version = "new"; close(mutated); return nil }); err != nil {
			panic(err)
		}
	}()
	select {
	case <-mutated:
		t.Fatal("rewind crossed subscription snapshot")
	case <-time.After(20 * time.Millisecond):
	}
	close(store.releaseVersion)
	sub := <-subscribed
	defer sub.Close()
	if event := receiveEvent(t, sub); event.Text != "old" {
		t.Fatalf("replay=%#v", event)
	}
	if event := receiveEvent(t, sub); event.HistoryVersion != "old" {
		t.Fatalf("version=%#v", event)
	}
	<-mutated
}

func TestIdleMutationBlocksNewRunUntilHistoryResetCompletes(t *testing.T) {
	hub := New(nil)
	entered, release := make(chan struct{}), make(chan struct{})
	mutationDone := make(chan error, 1)
	go func() { mutationDone <- hub.MutateIdle("abcd", func() error { close(entered); <-release; return nil }) }()
	<-entered
	started := make(chan bool, 1)
	go func() { _, ok := hub.StartRun("abcd", func() {}); started <- ok }()
	select {
	case <-started:
		t.Fatal("run began during rewind")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-mutationDone; err != nil {
		t.Fatal(err)
	}
	if !<-started {
		t.Fatal("run failed after rewind")
	}
	if err := hub.MutateIdle("abcd", func() error { t.Fatal("mutated a running chat"); return nil }); err != servicechat.ErrChatRunning {
		t.Fatal(err)
	}
}
