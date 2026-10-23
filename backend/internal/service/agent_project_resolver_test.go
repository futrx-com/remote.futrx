package service

import (
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"testing"
)

func TestProjectForAgentPreservesRoutingSlug(t *testing.T) {
	project := projectForAgent(serviceproject.Meta{
		ID: "project-id", Name: "Display Name", Slug: "routing-slug", ContainerName: "different-container",
	})
	if project.Slug != "routing-slug" || project.ContainerName != "different-container" {
		t.Fatalf("project routing identity lost: %#v", project)
	}
}
