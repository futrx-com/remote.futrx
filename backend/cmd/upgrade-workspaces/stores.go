package main

import (
	"fmt"

	serviceapps "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileapplications"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileproject"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileprojectaccess"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileprojectsecrets"
)

type workspaceStores struct {
	Projects       serviceproject.Repository
	ProjectSecrets serviceproject.SecretsRepository
	ProjectAccess  serviceproject.AccessRepository
	Applications   serviceapps.Store
}

// Workspace recycling runs while the backend owns conversation storage. Open
// only the project and application files required to rebuild its containers.
func newWorkspaceStores(dataDir, workspaceRoot string) (workspaceStores, error) {
	projects, err := fileproject.NewWithWorkspaceRoot(dataDir, workspaceRoot)
	if err != nil {
		return workspaceStores{}, fmt.Errorf("init project store: %w", err)
	}
	secrets, err := fileprojectsecrets.New(dataDir)
	if err != nil {
		return workspaceStores{}, fmt.Errorf("init project secrets store: %w", err)
	}
	access, err := fileprojectaccess.New(dataDir)
	if err != nil {
		return workspaceStores{}, fmt.Errorf("init project access store: %w", err)
	}
	applications, err := fileapplications.New(dataDir)
	if err != nil {
		return workspaceStores{}, fmt.Errorf("init applications store: %w", err)
	}
	return workspaceStores{
		Projects: projects, ProjectSecrets: secrets,
		ProjectAccess: access, Applications: applications,
	}, nil
}
