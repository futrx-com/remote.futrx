package image

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// shortenEgressRetry makes the retry loop finish in test time.
func shortenEgressRetry(t *testing.T) {
	t.Helper()
	deadline, interval := ipv4EgressDeadline, ipv4EgressInterval
	ipv4EgressDeadline = 60 * time.Millisecond
	ipv4EgressInterval = 5 * time.Millisecond
	t.Cleanup(func() {
		ipv4EgressDeadline, ipv4EgressInterval = deadline, interval
	})
}

// flakyProber fails a fixed number of times and then succeeds: a builder whose
// DHCP lease has not landed yet.
type flakyProber struct {
	failures int
	calls    int
}

func (p *flakyProber) ExecuteScript(context.Context, string, string) (string, error) {
	p.calls++
	if p.calls <= p.failures {
		return "", errors.New("exit 1")
	}
	return "", nil
}

// TestEgressProbeWaitsForASlowLease is the bug this change exists for. The
// probe ran once after a fixed warmup, so on a host where the lease takes
// longer it tested a container with no address and blamed Docker's FORWARD
// policy, sending the operator to inspect iptables on a machine without Docker.
func TestEgressProbeWaitsForASlowLease(t *testing.T) {
	shortenEgressRetry(t)
	prober := &flakyProber{failures: 3}

	if err := awaitIPv4Egress(context.Background(), prober, "builder"); err != nil {
		t.Fatalf("awaitIPv4Egress() = %v, want it to wait for the lease", err)
	}
	if prober.calls != 4 {
		t.Fatalf("probed %d times, want retries until the address arrives", prober.calls)
	}
}

// TestEgressProbeStillReportsARealBlock: waiting must not turn a genuine
// failure into a pass. The operator still needs the hint that names the cause.
func TestEgressProbeStillReportsARealBlock(t *testing.T) {
	shortenEgressRetry(t)
	prober := &flakyProber{failures: 1 << 30}

	err := awaitIPv4Egress(context.Background(), prober, "builder")
	if err == nil {
		t.Fatal("awaitIPv4Egress() = nil, want the egress hint")
	}
	for _, want := range []string{"cannot reach any IPv4", "Docker", "DOCKER-USER"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if prober.calls < 2 {
		t.Errorf("probed %d times, want more than one attempt before giving up", prober.calls)
	}
}

// TestEgressProbeReportsCancellationAsCancellation: an interrupted build is not
// a network fault, and pointing the operator at iptables because they pressed
// Ctrl-C would be a lie.
func TestEgressProbeReportsCancellationAsCancellation(t *testing.T) {
	shortenEgressRetry(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := awaitIPv4Egress(ctx, &flakyProber{failures: 1 << 30}, "builder"); !errors.Is(err, context.Canceled) {
		t.Fatalf("awaitIPv4Egress() = %v, want context.Canceled", err)
	}
}
