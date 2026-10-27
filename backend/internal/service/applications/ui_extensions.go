package applications

import (
	"context"
	"slices"
)

// UIExtension pairs an installed application UI with the scopes where it applies.
type UIExtension struct {
	Application Application `json:"application"`
	Global      bool        `json:"global"`
	ProjectIDs  []string    `json:"projectIds,omitempty"`
	// Backends are the running instances of this application whose backend the
	// extension may call. An application installed both globally and in a project
	// has one backend process per install, so an extension addresses an
	// instance, not an application.
	Backends []BackendInstance `json:"backends,omitempty"`
}

// UIAsset returns one embedded application UI asset. Authentication and response
// delivery remain transport responsibilities.
func (s *Service) UIAsset(applicationID, assetPath string) ([]byte, bool) {
	return s.registry.UIAsset(applicationID, assetPath)
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
	registry      Registry
	byApplication map[string]*UIExtension
	order         []string
}

func newUIExtensionAccumulator(registry Registry) *uiExtensionAccumulator {
	return &uiExtensionAccumulator{
		registry:      registry,
		byApplication: make(map[string]*UIExtension),
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
	application, ok := a.registry.Get(instance.ApplicationID)
	if !ok || application.UI == nil {
		return
	}
	extension, exists := a.byApplication[instance.ApplicationID]
	if !exists {
		extension = &UIExtension{Application: application}
		a.byApplication[instance.ApplicationID] = extension
		a.order = append(a.order, instance.ApplicationID)
	}
	a.addBackend(extension, application, instance)
	if projectID == "" {
		extension.Global = true
		return
	}
	if !slices.Contains(extension.ProjectIDs, projectID) {
		extension.ProjectIDs = append(extension.ProjectIDs, projectID)
	}
}

// addBackend records the instance an extension should address for a backend
// call. The same instance can be reached through both the global list and a
// project list, so the entry is deduplicated by instance id.
func (a *uiExtensionAccumulator) addBackend(extension *UIExtension, application Application, instance Instance) {
	if application.Backend == nil {
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
	for _, applicationID := range a.order {
		extensions = append(extensions, *a.byApplication[applicationID])
	}
	return extensions
}
