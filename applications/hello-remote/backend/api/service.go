package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

const serviceInspectionTimeout = 5 * time.Second

type serviceHealth struct {
	Status             string `json:"status"`
	Message            string `json:"message"`
	Version            string `json:"version"`
	ProvisionedVersion string `json:"provisionedVersion"`
}

type serviceInfo struct {
	Status             string `json:"status"`
	Message            string `json:"message"`
	Version            string `json:"version"`
	ProvisionedVersion string `json:"provisionedVersion"`
	Service            string `json:"service"`
	InternalPort       int    `json:"internalPort"`
	ExternalPort       int    `json:"externalPort"`
}

// readServiceHealth crosses the LXD proxy that Remote created from the declared
// port. It proves the manifest, guest service, host port, and backend can work
// together without teaching the backend how Remote controls LXD.
func readServiceHealth(externalPort int) (serviceHealth, error) {
	client := http.Client{Timeout: serviceInspectionTimeout}
	response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", externalPort))
	if err != nil {
		return serviceHealth{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return serviceHealth{}, fmt.Errorf("health endpoint returned %s", response.Status)
	}
	var health serviceHealth
	if err := json.NewDecoder(response.Body).Decode(&health); err != nil {
		return serviceHealth{}, fmt.Errorf("decode health response: %w", err)
	}
	return health, nil
}

func (b *api) service(applications.Request) applications.Response {
	b.mu.Lock()
	service := b.instance.Service
	internalPort := b.instance.InternalPort
	externalPort := b.instance.ExternalPort
	inspect := b.inspectService
	b.mu.Unlock()

	if externalPort == 0 {
		return applications.JSON(http.StatusConflict, map[string]string{
			"error": "this install has no exposed service port",
		})
	}
	health, err := inspect(externalPort)
	if err != nil {
		return applications.JSON(http.StatusBadGateway, map[string]string{
			"error": fmt.Sprintf("could not reach the container service: %v", err),
		})
	}
	return applications.JSON(http.StatusOK, serviceInfo{
		Status:             health.Status,
		Message:            health.Message,
		Version:            health.Version,
		ProvisionedVersion: health.ProvisionedVersion,
		Service:            service,
		InternalPort:       internalPort,
		ExternalPort:       externalPort,
	})
}
