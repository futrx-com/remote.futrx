package applications

import (
	"context"
	"slices"
)

// UIExtension pairs an installed image UI with the scopes where it applies.
type UIExtension struct {
	Image      Image    `json:"image"`
	Global     bool     `json:"global"`
	ProjectIDs []string `json:"projectIds,omitempty"`
}

// UIAsset returns one embedded image UI asset. Authentication and response
// delivery remain transport responsibilities.
func (s *Service) UIAsset(imageID, assetPath string) ([]byte, bool) {
	return s.registry.UIAsset(imageID, assetPath)
}

// UIExtensions returns running extensions installed globally or in one of the
// caller-visible projects. The transport supplies that visibility boundary.
func (s *Service) UIExtensions(ctx context.Context, projectIDs []string) ([]UIExtension, error) {
	accumulator := newUIExtensionAccumulator(s.registry)

	globals, err := s.store.ListGlobal(ctx)
	if err != nil {
		return nil, err
	}
	accumulator.addAll(globals, "")

	for _, projectID := range projectIDs {
		instances, err := s.store.ListProject(ctx, projectID)
		if err != nil {
			return nil, err
		}
		accumulator.addAll(instances, projectID)
	}
	return accumulator.extensions(), nil
}

// uiExtensionAccumulator owns the union and stable-order invariants while the
// service remains responsible for loading instances from their scopes.
type uiExtensionAccumulator struct {
	registry Registry
	byImage  map[string]*UIExtension
	order    []string
}

func newUIExtensionAccumulator(registry Registry) *uiExtensionAccumulator {
	return &uiExtensionAccumulator{
		registry: registry,
		byImage:  make(map[string]*UIExtension),
	}
}

func (a *uiExtensionAccumulator) addAll(instances []Instance, projectID string) {
	for _, instance := range instances {
		a.add(instance, projectID)
	}
}

func (a *uiExtensionAccumulator) add(instance Instance, projectID string) {
	if instance.Status != StatusRunning {
		return
	}
	image, ok := a.registry.Get(instance.ImageID)
	if !ok || image.UI == nil {
		return
	}
	extension, exists := a.byImage[instance.ImageID]
	if !exists {
		extension = &UIExtension{Image: image}
		a.byImage[instance.ImageID] = extension
		a.order = append(a.order, instance.ImageID)
	}
	if projectID == "" {
		extension.Global = true
		return
	}
	if !slices.Contains(extension.ProjectIDs, projectID) {
		extension.ProjectIDs = append(extension.ProjectIDs, projectID)
	}
}

func (a *uiExtensionAccumulator) extensions() []UIExtension {
	extensions := make([]UIExtension, 0, len(a.order))
	for _, imageID := range a.order {
		extensions = append(extensions, *a.byImage[imageID])
	}
	return extensions
}
