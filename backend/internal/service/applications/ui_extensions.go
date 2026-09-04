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
	// Backends are the running instances of this image whose plugin the
	// extension may call. An image installed both globally and in a project
	// has one plugin process per install, so an extension addresses an
	// instance, not an image.
	Backends []BackendInstance `json:"backends,omitempty"`
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
	a.addBackend(extension, image, instance)
	if projectID == "" {
		extension.Global = true
		return
	}
	if !slices.Contains(extension.ProjectIDs, projectID) {
		extension.ProjectIDs = append(extension.ProjectIDs, projectID)
	}
}

// addBackend records the instance an extension should address for a plugin
// call. The same instance can be reached through both the global list and a
// project list, so the entry is deduplicated by instance id.
func (a *uiExtensionAccumulator) addBackend(extension *UIExtension, image Image, instance Instance) {
	if image.Backend == nil {
		return
	}
	for _, existing := range extension.Backends {
		if existing.InstanceID == instance.ID {
			return
		}
	}
	extension.Backends = append(extension.Backends, BackendInstance{
		InstanceID: instance.ID,
		Scope:      instance.Scope,
		ProjectID:  instance.ProjectID,
	})
}

func (a *uiExtensionAccumulator) extensions() []UIExtension {
	extensions := make([]UIExtension, 0, len(a.order))
	for _, imageID := range a.order {
		extensions = append(extensions, *a.byImage[imageID])
	}
	return extensions
}
