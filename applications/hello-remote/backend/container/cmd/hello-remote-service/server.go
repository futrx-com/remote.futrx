package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

type healthResponse struct {
	Status             string `json:"status"`
	Message            string `json:"message"`
	Version            string `json:"version"`
	ProvisionedVersion string `json:"provisionedVersion"`
}

func serve(port int, getenv func(string) string) error {
	config, err := configurationFromEnv(getenv)
	if err != nil {
		return err
	}
	config.ProvisionedVersion, err = readProvisionedVersion(provisionedVersionPath)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:              "0.0.0.0:" + strconv.Itoa(port),
		Handler:           serviceHandler(config),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return server.ListenAndServe()
}

func serviceHandler(config configuration) http.Handler {
	router := http.NewServeMux()
	handle := func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(healthResponse{
			Status:             "ok",
			Message:            config.Greeting + " from the container service.",
			Version:            version,
			ProvisionedVersion: config.ProvisionedVersion,
		})
	}
	router.HandleFunc("/", handle)
	router.HandleFunc("/health", handle)
	return router
}
