package usersettings

import (
	"context"
	"errors"
	"testing"

	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
)

func TestDefaultSettingsUseCodexChatDefaults(t *testing.T) {
	settings := DefaultSettings()
	if settings.Chat.Provider != ChatProviderCodex {
		t.Fatalf("expected codex default provider, got %q", settings.Chat.Provider)
	}
	if settings.Chat.Mode != ChatModeDefault {
		t.Fatalf("expected provider default mode, got %q", settings.Chat.Mode)
	}
	if settings.Chat.Model != "" || settings.Chat.ReasoningEffort != "" {
		t.Fatalf("expected auto model and reasoning effort, got %+v", settings.Chat)
	}
}

func TestUpdatePersistsChatPreferences(t *testing.T) {
	repo := &memoryRepo{}
	service := New(repo)
	provider := ChatProviderClaude
	model := " sonnet "
	mode := ChatModePlan
	effort := ReasoningEffortHigh

	settings, err := service.Update(context.Background(), "sub:user", UpdateInput{
		Chat: &ChatUpdate{
			Provider:        &provider,
			Model:           &model,
			Mode:            &mode,
			ReasoningEffort: &effort,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if settings.Chat.Provider != ChatProviderClaude || settings.Chat.Model != "sonnet" || settings.Chat.Mode != ChatModePlan || settings.Chat.ReasoningEffort != ReasoningEffortHigh {
		t.Fatalf("unexpected chat settings: %+v", settings.Chat)
	}
	if !repo.saved {
		t.Fatal("expected settings to be saved")
	}
}

func TestUpdatePreservesProviderDefinedCapabilityValues(t *testing.T) {
	repo := &memoryRepo{}
	effort := ReasoningEffort(" Future.V2 ")
	tier := ServiceTier(" Burst_2 ")

	settings, err := New(repo).Update(context.Background(), "sub:user", UpdateInput{
		Chat: &ChatUpdate{ReasoningEffort: &effort, ServiceTier: &tier},
	})
	if err != nil {
		t.Fatal(err)
	}
	if settings.Chat.ReasoningEffort != "Future.V2" || settings.Chat.ServiceTier != "Burst_2" {
		t.Fatalf("provider capability values were changed: %+v", settings.Chat)
	}
}

func TestUpdateAcceptsFutureProviderIdentifiers(t *testing.T) {
	provider := ChatProvider("future-agent")
	settings, err := New(&memoryRepo{}).Update(context.Background(), "sub:user", UpdateInput{
		Chat: &ChatUpdate{Provider: &provider},
	})
	if err != nil {
		t.Fatal(err)
	}
	if settings.Chat.Provider != provider {
		t.Fatalf("provider = %q, want %q", settings.Chat.Provider, provider)
	}
}

func TestUpdateRejectsInvalidChatPreferences(t *testing.T) {
	tests := []struct {
		name string
		in   UpdateInput
		want error
	}{
		{
			name: "provider",
			in: UpdateInput{Chat: &ChatUpdate{
				Provider: ptr(ChatProvider("bad provider")),
			}},
			want: ErrInvalidChatProvider,
		},
		{
			name: "mode",
			in: UpdateInput{Chat: &ChatUpdate{
				Mode: ptr(ChatMode("bad")),
			}},
			want: ErrInvalidChatMode,
		},
		{
			name: "reasoning effort",
			in: UpdateInput{Chat: &ChatUpdate{
				ReasoningEffort: ptr(ReasoningEffort("bad value")),
			}},
			want: ErrInvalidReasoningEffort,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(&memoryRepo{}).Update(context.Background(), "sub:user", tt.in)
			if !errors.Is(err, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, err)
			}
		})
	}
}

type testProviderCatalog map[string]bool

func (c testProviderCatalog) HasProvider(provider string) bool { return c[provider] }
func (c testProviderCatalog) SupportsScope(provider string, _ agentmodule.ExecutionScope) bool {
	return c[provider]
}

type defaultTestProviderCatalog struct {
	testProviderCatalog
	provider ChatProvider
}

func (c defaultTestProviderCatalog) DefaultProvider(agentmodule.ExecutionScope) ChatProvider {
	return c.provider
}

func TestGetUsesConfiguredDefaultProvider(t *testing.T) {
	service := New(
		&memoryRepo{},
		WithProviderCatalog(defaultTestProviderCatalog{
			testProviderCatalog: testProviderCatalog{"future-agent": true},
			provider:            "future-agent",
		}),
	)
	settings, err := service.Get(context.Background(), "sub:user")
	if err != nil {
		t.Fatal(err)
	}
	if settings.Chat.Provider != "future-agent" {
		t.Fatalf("default provider = %q, want future-agent", settings.Chat.Provider)
	}
}

func TestUpdateRejectsProviderMissingFromConfiguredCatalog(t *testing.T) {
	provider := ChatProvider("future-agent")
	_, err := New(
		&memoryRepo{},
		WithProviderCatalog(testProviderCatalog{"codex": true}),
	).Update(context.Background(), "sub:user", UpdateInput{Chat: &ChatUpdate{Provider: &provider}})
	if !errors.Is(err, ErrInvalidChatProvider) {
		t.Fatalf("Update error = %v, want ErrInvalidChatProvider", err)
	}
}

func ptr[T any](v T) *T {
	return &v
}

type memoryRepo struct {
	settings Settings
	exists   bool
	saved    bool
}

func (r *memoryRepo) Get(context.Context, Key) (Settings, error) {
	if !r.exists {
		return Settings{}, ErrNotFound
	}
	return r.settings, nil
}

func (r *memoryRepo) Save(_ context.Context, _ Key, settings Settings) (Settings, error) {
	r.settings = settings
	r.exists = true
	r.saved = true
	return settings, nil
}
