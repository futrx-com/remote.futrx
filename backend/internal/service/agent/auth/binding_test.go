package auth

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

type bindingTestStatus struct {
	Authenticated bool        `json:"authenticated"`
	DeviceLogin   DeviceState `json:"deviceLogin"`
}

func TestDeviceBindingPreservesConcreteStatus(t *testing.T) {
	authenticated := true
	service := NewDeviceService(DeviceConfig[bindingTestStatus]{
		Authenticated: func() bool { return authenticated },
		BuildStatus: func() DeviceStatusBuilder[bindingTestStatus] {
			return func(state DeviceState) bindingTestStatus {
				return bindingTestStatus{Authenticated: authenticated, DeviceLogin: state}
			}
		},
	})
	binding := NewDeviceBinding(agent.ProviderCodex, service)

	if binding.ID() != agent.ProviderCodex || binding.Flow() != FlowDevice {
		t.Fatalf("binding identity = (%q, %q)", binding.ID(), binding.Flow())
	}
	if !binding.Authenticated() {
		t.Fatal("authenticated service was reported as unauthenticated")
	}
	if _, ok := binding.Status().(bindingTestStatus); !ok {
		t.Fatalf("status type = %T, want bindingTestStatus", binding.Status())
	}

	subscription, err := binding.Subscribe()
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Close()

	status, ok := subscription.Next(context.Background())
	if !ok {
		t.Fatal("initial status was not delivered")
	}
	if _, ok := status.(bindingTestStatus); !ok {
		t.Fatalf("streamed status type = %T, want bindingTestStatus", status)
	}
}

func TestBindingsExposeNormalizedSnapshots(t *testing.T) {
	codeService := NewCodeService(CodeConfig{Authenticated: func() bool { return true }})
	codeService.state = CodeLoginState{
		Active: true, AuthURL: "https://login.example/code", AwaitingCode: true, StartedAt: 11,
	}
	code := NewCodeBinding("future-code", codeService)
	codeSnapshot := code.Snapshot()
	if !codeSnapshot.Authenticated || !codeSnapshot.Login.Active ||
		codeSnapshot.Login.URL != "https://login.example/code" || !codeSnapshot.Login.AwaitingCode {
		t.Fatalf("code snapshot = %#v", codeSnapshot)
	}

	deviceService := NewDeviceService(DeviceConfig[bindingTestStatus]{
		Authenticated: func() bool { return false },
		BuildStatus: func() DeviceStatusBuilder[bindingTestStatus] {
			return func(state DeviceState) bindingTestStatus {
				return bindingTestStatus{DeviceLogin: state}
			}
		},
	})
	deviceService.device = DeviceState{
		Active: true, VerificationURI: "https://login.example/device",
		UserCode: "ABCD-EFGH", StartedAt: 12, ExpiresAt: 34,
	}
	device := NewDeviceBinding("future-device", deviceService).WithWarning(func() string {
		return "Use subscription authentication."
	})
	deviceSnapshot := device.Snapshot()
	if deviceSnapshot.Authenticated || !deviceSnapshot.Login.Active ||
		deviceSnapshot.Login.URL != "https://login.example/device" ||
		deviceSnapshot.Login.UserCode != "ABCD-EFGH" ||
		deviceSnapshot.Warning != "Use subscription authentication." {
		t.Fatalf("device snapshot = %#v", deviceSnapshot)
	}

	subscription, err := device.SubscribeSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Close()
	value, ok := subscription.Next(context.Background())
	if !ok {
		t.Fatal("normalized initial snapshot was not delivered")
	}
	if got, ok := value.(Snapshot); !ok || got.Login.UserCode != "ABCD-EFGH" || got.Warning == "" {
		t.Fatalf("normalized stream value = %#v", value)
	}
}

func TestExternalBindingDeclaresProviderOwnedAuthentication(t *testing.T) {
	binding := NewExternalBinding(agent.ProviderAntigravity)
	if binding.ID() != agent.ProviderAntigravity || binding.Flow() != FlowExternal {
		t.Fatalf("binding identity = (%q, %q)", binding.ID(), binding.Flow())
	}
	if binding.Available() || binding.Authenticated() || binding.Status() != nil {
		t.Fatalf("external binding unexpectedly exposes managed auth: %#v", binding)
	}
	if snapshot := binding.Snapshot(); snapshot.Authenticated || snapshot.Login.Active {
		t.Fatalf("external snapshot = %#v", snapshot)
	}
	if _, err := binding.SubscribeSnapshots(); !errors.Is(err, ErrUnsupportedFlow) {
		t.Fatalf("SubscribeSnapshots error = %v, want ErrUnsupportedFlow", err)
	}
	if _, err := binding.Subscribe(); !errors.Is(err, ErrUnsupportedFlow) {
		t.Fatalf("Subscribe error = %v, want ErrUnsupportedFlow", err)
	}
}

func TestCodeBindingClassifiesConfiguredInputErrors(t *testing.T) {
	required := errors.New("code required")
	noSession := errors.New("no session")
	binding := NewCodeBinding(agent.ProviderClaude, NewCodeService(CodeConfig{
		CodeRequired: required,
		NoSession:    noSession,
	}))

	for _, err := range []error{required, fmt.Errorf("wrapped: %w", noSession)} {
		if !binding.IsCodeInputError(err) {
			t.Fatalf("IsCodeInputError(%v) = false", err)
		}
	}
	if binding.IsCodeInputError(errors.New("internal")) {
		t.Fatal("unexpected error classified as caller input")
	}
}

func TestUnavailableBindingRejectsSubscription(t *testing.T) {
	binding := NewCodeBinding(agent.ProviderClaude, nil)
	if binding.Available() {
		t.Fatal("nil service binding is available")
	}
	if _, err := binding.Subscribe(); !errors.Is(err, ErrUnsupportedFlow) {
		t.Fatalf("Subscribe error = %v, want ErrUnsupportedFlow", err)
	}
}
