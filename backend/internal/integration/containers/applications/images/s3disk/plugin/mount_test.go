package main

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
)

func TestStatusUsesInstalledContainerAndRedactsFailures(t *testing.T) {
	b := testBackend(t)
	b.run = func(_ context.Context, container string, args ...string) commandResult {
		if container != "project-test" {
			t.Fatalf("unexpected container %s", container)
		}
		return commandResult{Output: "private-value", Error: "failed private-value"}
	}
	response, _ := b.Handle(appplugin.Request{Method: "GET", Path: "status", Body: []byte(`{"container":"other"}`)})
	if strings.Contains(string(response.Body), "private-value") {
		t.Fatal("credential leaked")
	}
	var status struct{ Mounted bool }
	if err := json.Unmarshal(response.Body, &status); err != nil {
		t.Fatal(err)
	}
	if status.Mounted {
		t.Fatal("failed check reported mounted")
	}
}

func TestOperationsSerializeAndReportFailures(t *testing.T) {
	b := testBackend(t)
	release := make(chan struct{})
	b.run = func(_ context.Context, container string, args ...string) commandResult {
		if !reflect.DeepEqual(args, []string{"/usr/local/bin/s3disk", "sync", "/workspace/s3"}) {
			t.Errorf("unexpected args %v", args)
		}
		<-release
		return commandResult{Error: "upload failed"}
	}
	result, _ := b.Handle(appplugin.Request{Method: "POST", Path: "sync"})
	if result.Status != http.StatusAccepted {
		t.Fatalf("status %d", result.Status)
	}
	result, _ = b.Handle(appplugin.Request{Method: "POST", Path: "restart"})
	if result.Status != http.StatusConflict {
		t.Fatalf("concurrent restart: %d", result.Status)
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		b.mu.Lock()
		op := b.operation
		b.mu.Unlock()
		if !op.Running {
			if op.Result.Error != "upload failed" {
				t.Fatalf("lost failure: %+v", op)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("operation did not finish")
}

func TestRejectsInvalidTargetsAndUnknownActions(t *testing.T) {
	for _, name := range []string{"", "--help", "remote:other", "a;sh"} {
		b := newBackend()
		if b.Init(appplugin.Instance{Scope: "project", ContainerName: name}) == nil {
			t.Errorf("accepted %q", name)
		}
	}
	b := testBackend(t)
	b.run = func(context.Context, string, ...string) commandResult {
		t.Fatal("unexpected command")
		return commandResult{}
	}
	for _, request := range []appplugin.Request{{Method: "POST", Path: "exec"}, {Method: "GET", Path: "sync"}} {
		response, _ := b.Handle(request)
		if response.Status < 400 {
			t.Fatalf("accepted %+v", request)
		}
	}
}
