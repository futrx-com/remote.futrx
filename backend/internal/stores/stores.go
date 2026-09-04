package stores

import (
	"context"
	"fmt"

	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	servicepush "github.com/futrx-com/remote.futrx.com/internal/service/push"
	serviceschedule "github.com/futrx-com/remote.futrx.com/internal/service/schedule"
	serviceusage "github.com/futrx-com/remote.futrx.com/internal/service/usage"
	serviceuser "github.com/futrx-com/remote.futrx.com/internal/service/user"
	serviceusersettings "github.com/futrx-com/remote.futrx.com/internal/service/usersettings"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileauth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filechat"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileproject"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileprojectaccess"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileprojectsecrets"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filepush"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileschedule"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filesessions"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filetwofactor"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileusage"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileusers"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileusersettings"
)

type AuthStore interface {
	serviceauth.Store
}

// ChatStore retains the complete file-chat capability until composition can
// project it into each service's narrower repository and transcript contracts.
type ChatStore interface {
	servicechat.Repository
	servicechat.TranscriptEventSource
}

// PushStore exposes the subscription, account-cleanup, and VAPID capabilities
// required at the application composition boundary.
type PushStore interface {
	servicepush.Repository
	DeleteAll(ctx context.Context, email string) error
	VAPIDKeys(generate func() (private string, public string, err error)) (string, string, error)
}

type Stores struct {
	Chats           ChatStore
	Projects        serviceproject.Repository
	ProjectSecrets  serviceproject.SecretsRepository
	ProjectAccess   serviceproject.AccessRepository
	Schedules       serviceschedule.Repository
	Auth            AuthStore
	Users           serviceuser.Repository
	UserSettings    serviceusersettings.Repository
	TwoFactor       serviceauth.TwoFactorStore
	SessionRegistry serviceauth.SessionRegistryStore
	Push            PushStore
	Usage           serviceusage.Repository
	AgentAPIKeys    agentauth.APIKeyStore
}

func New(dataDir string) (Stores, error) {
	chats, err := filechat.New(dataDir)
	if err != nil {
		return Stores{}, fmt.Errorf("init chat store: %w", err)
	}

	projects, err := fileproject.New(dataDir)
	if err != nil {
		return Stores{}, fmt.Errorf("init project store: %w", err)
	}

	projectSecrets, err := fileprojectsecrets.New(dataDir)
	if err != nil {
		return Stores{}, fmt.Errorf("init project secrets store: %w", err)
	}

	projectAccess, err := fileprojectaccess.New(dataDir)
	if err != nil {
		return Stores{}, fmt.Errorf("init project access store: %w", err)
	}

	schedules, err := fileschedule.New(dataDir)
	if err != nil {
		return Stores{}, fmt.Errorf("init scheduled tasks store: %w", err)
	}

	users, err := fileusers.New(dataDir)
	if err != nil {
		return Stores{}, fmt.Errorf("init users store: %w", err)
	}

	userSettings, err := fileusersettings.New(dataDir)
	if err != nil {
		return Stores{}, fmt.Errorf("init user settings store: %w", err)
	}

	twoFactor, err := filetwofactor.New(dataDir)
	if err != nil {
		return Stores{}, fmt.Errorf("init two-factor store: %w", err)
	}

	sessionRegistry, err := filesessions.New(dataDir)
	if err != nil {
		return Stores{}, fmt.Errorf("init session registry store: %w", err)
	}

	usage, err := fileusage.New(dataDir)
	if err != nil {
		return Stores{}, fmt.Errorf("init usage store: %w", err)
	}

	push, err := filepush.New(dataDir)
	if err != nil {
		return Stores{}, fmt.Errorf("init push subscriptions store: %w", err)
	}

	authStore := fileauth.New(dataDir)
	return Stores{
		Chats:           chats,
		Projects:        projects,
		ProjectSecrets:  projectSecrets,
		ProjectAccess:   projectAccess,
		Schedules:       schedules,
		Auth:            authStore,
		Users:           users,
		UserSettings:    userSettings,
		TwoFactor:       twoFactor,
		SessionRegistry: sessionRegistry,
		Push:            push,
		Usage:           usage,
		AgentAPIKeys:    authStore,
	}, nil
}
