package stores

import (
	"fmt"

	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	serviceschedule "github.com/futrx-com/remote.futrx.com/internal/service/schedule"
	serviceshare "github.com/futrx-com/remote.futrx.com/internal/service/share"
	serviceuser "github.com/futrx-com/remote.futrx.com/internal/service/user"
	serviceusersettings "github.com/futrx-com/remote.futrx.com/internal/service/usersettings"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileauth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filechat"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileproject"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileprojectaccess"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileprojectsecrets"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileprojectshares"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileschedule"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileusers"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileusersettings"
)

type AuthStore interface {
	serviceauth.Store
}

type Stores struct {
	Chats          servicechat.Repository
	Projects       serviceproject.Repository
	ProjectSecrets serviceproject.SecretsRepository
	ProjectAccess  serviceproject.AccessRepository
	ProjectShares  serviceshare.Repository
	Schedules      serviceschedule.Repository
	Auth           AuthStore
	Users          serviceuser.Repository
	UserSettings   serviceusersettings.Repository
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

	projectShares, err := fileprojectshares.New(dataDir)
	if err != nil {
		return Stores{}, fmt.Errorf("init project shares store: %w", err)
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

	return Stores{
		Chats:          chats,
		Projects:       projects,
		ProjectSecrets: projectSecrets,
		ProjectAccess:  projectAccess,
		ProjectShares:  projectShares,
		Schedules:      schedules,
		Auth:           fileauth.New(dataDir),
		Users:          users,
		UserSettings:   userSettings,
	}, nil
}
