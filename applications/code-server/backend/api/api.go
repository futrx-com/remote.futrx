package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"futrx.local/catalog/applications/code-server/backend/config"
	appsettings "futrx.local/catalog/applications/code-server/backend/settings"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

type settingsService interface {
	Init(applications.Instance) error
	Read() ([]byte, error)
	Save([]byte) ([]byte, error)
}

// API owns post-install settings. The install script only seeds and restores
// the files; browser edits always pass through this backend.
type API struct {
	router   *applications.Router
	mu       sync.Mutex
	settings settingsService
}

var _ applications.Backend = (*API)(nil)

func New(settings settingsService) *API {
	b := &API{router: applications.NewRouter(), settings: settings}
	b.router.GET("settings", "Read the active Code Server settings", b.getSettings)
	b.router.POST("settings", "Save Code Server settings", b.saveSettings)
	return b
}

func (b *API) Describe() (applications.Descriptor, error) {
	return applications.Descriptor{APIVersion: applications.APIVersion, Routes: b.router.Routes()}, nil
}

func (b *API) Init(instance applications.Instance) error {
	return b.settings.Init(instance)
}

func (b *API) Handle(request applications.Request) (applications.Response, error) {
	return b.router.Serve(request), nil
}

func (b *API) getSettings(_ applications.Request) applications.Response {
	b.mu.Lock()
	defer b.mu.Unlock()
	settings, err := b.settings.Read()
	if err != nil {
		var invalid *appsettings.InvalidStoredError
		if errors.As(err, &invalid) {
			return applications.JSON(http.StatusBadGateway, map[string]string{"error": "Code Server settings on disk are invalid: " + err.Error()})
		}
		return applications.JSON(http.StatusBadGateway, map[string]string{"error": "Could not read Code Server settings: " + err.Error()})
	}
	return applications.JSON(http.StatusOK, map[string]string{"settings": string(settings)})
}

func (b *API) saveSettings(request applications.Request) applications.Response {
	b.mu.Lock()
	defer b.mu.Unlock()
	var input struct {
		Settings string `json:"settings"`
	}
	if len(request.Body) > config.MaxSettingsBytes+1024 || json.Unmarshal(request.Body, &input) != nil {
		return applications.JSON(http.StatusBadRequest, map[string]string{"error": "Send a settings JSON document."})
	}
	settings, err := b.settings.Save([]byte(input.Settings))
	if err != nil {
		var invalid *appsettings.InvalidInputError
		if errors.As(err, &invalid) {
			return applications.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return applications.JSON(http.StatusBadGateway, map[string]string{"error": "Could not save Code Server settings: " + err.Error()})
	}
	return applications.JSON(http.StatusOK, map[string]string{"settings": string(settings)})
}
