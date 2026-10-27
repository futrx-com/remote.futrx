package applications

import (
	"context"
	"errors"
	"testing"
	"time"

	api "github.com/futrx-com/remote.futrx.com/pkg/applications"
)

func TestExpiredApplicationGrantIsRemovedBeforeDispatch(t *testing.T) {
	now := time.Now()
	s := &Service{agentRuntime: &agentRuntime{}, agentTools: &applicationTools{now: func() time.Time { return now }, grants: map[string]applicationGrant{"expired": {expires: now}}}}
	if _, err := s.CallAgentApplication(context.Background(), "expired", "anything", api.Request{}); !errors.Is(err, ErrInvalidAgentGrant) {
		t.Fatalf("expired grant: %v", err)
	}
	if len(s.agentTools.grants) != 0 {
		t.Fatal("expired grant retained")
	}
}
