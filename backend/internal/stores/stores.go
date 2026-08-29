package stores

import (
	"context"
	"fmt"

	serviceagentquota "github.com/futrx-com/remote.futrx.com/internal/service/agentquota"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	servicepush "github.com/futrx-com/remote.futrx.com/internal/service/push"
	serviceschedule "github.com/futrx-com/remote.futrx.com/internal/service/schedule"
	serviceusage "github.com/futrx-com/remote.futrx.com/internal/service/usage"
	serviceuser "github.com/futrx-com/remote.futrx.com/internal/service/user"
	serviceusersettings "github.com/futrx-com/remote.futrx.com/internal/service/usersettings"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileagentquota"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileauth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filechat"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileproject"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileprojectaccess"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileprojectsecrets"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filepush"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileschedule"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileusage"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileusers"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileusersettings"
)

type AuthStore interface {
	serviceauth.Store
}

// PushStore exposes the subscription, account-cleanup, and VAPID capabilities
// required at the application composition boundary.
type PushStore interface {
	servicepush.Repository
	DeleteAll(ctx context.Context, email string) error
	VAPIDKeys(generate func() (private string, public string, err error)) (string, string, error)
}

type Stores struct {
	Chats          servicechat.Repository
	Projects       serviceproject.Repository
	ProjectSecrets serviceproject.SecretsRepository
	ProjectAccess  serviceproject.AccessRepository
	Schedules      serviceschedule.Repository
	Auth           AuthStore
	Users          serviceuser.Repository
	UserSettings   serviceusersettings.Repository
	Push           PushStore
	Usage          serviceusage.Repository
	AgentQuota     serviceagentquota.Repository
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

	usage, err := fileusage.New(dataDir)
	if err != nil {
		return Stores{}, fmt.Errorf("init usage store: %w", err)
	}
	agentQuota, err := fileagentquota.New(dataDir)
	if err != nil {
		return Stores{}, fmt.Errorf("init agent quota store: %w", err)
	}
	push, err := filepush.New(dataDir)
	if err != nil {
		return Stores{}, fmt.Errorf("init push subscriptions store: %w", err)
	}

	return Stores{
		Chats:          chats,
		Projects:       projects,
		ProjectSecrets: projectSecrets,
		ProjectAccess:  projectAccess,
		Schedules:      schedules,
		Auth:           fileauth.New(dataDir),
		Users:          users,
		UserSettings:   userSettings,
		Push:           push,
		Usage:          usage,
		AgentQuota:     agentQuota,
	}, nil
}
