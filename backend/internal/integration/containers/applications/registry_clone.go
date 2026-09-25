package applications

import (
	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	applicationapi "github.com/futrx-com/remote.futrx.com/pkg/applications"
)

// cloneApplication keeps the registry's catalog snapshot immutable to callers.
// Application contains slices, maps, and pointers at several levels, so a
// struct copy alone would let a List or Get caller change later reads.
func cloneApplication(application svc.Application) svc.Application {
	cloned := application
	cloned.HostTools = cloneHostTools(application.HostTools)
	cloned.Scopes = cloneSlice(application.Scopes)
	cloned.Env = cloneSlice(application.Env)
	cloned.Service = cloneApplicationService(application.Service)
	cloned.UI = cloneApplicationUI(application.UI)
	cloned.Backend = cloneApplicationBackend(application.Backend)
	cloned.Publishers = clonePublishers(application.Publishers)
	cloned.Subscriptions = cloneSubscriptions(application.Subscriptions)
	cloned.Container = cloneApplicationContainer(application.Container)
	cloned.Skills = cloneSlice(application.Skills)
	return cloned
}

func cloneHostTools(tools []svc.HostTool) []svc.HostTool {
	if tools == nil {
		return nil
	}
	cloned := make([]svc.HostTool, len(tools))
	for i, tool := range tools {
		cloned[i] = tool
		cloned[i].VersionArgs = cloneSlice(tool.VersionArgs)
		if tool.Downloads != nil {
			cloned[i].Downloads = make(map[string]svc.HostToolDownload, len(tool.Downloads))
			for architecture, download := range tool.Downloads {
				cloned[i].Downloads[architecture] = download
			}
		}
	}
	return cloned
}

func cloneApplicationService(service *svc.ApplicationService) *svc.ApplicationService {
	if service == nil {
		return nil
	}
	cloned := *service
	cloned.Command = cloneSlice(service.Command)
	cloned.Environment = cloneSlice(service.Environment)
	return &cloned
}

func cloneApplicationUI(ui *svc.ApplicationUI) *svc.ApplicationUI {
	if ui == nil {
		return nil
	}
	cloned := *ui
	cloned.Styles = cloneSlice(ui.Styles)
	if ui.Views != nil {
		cloned.Views = make(map[string]string, len(ui.Views))
		for name, file := range ui.Views {
			cloned.Views[name] = file
		}
	}
	return &cloned
}

func cloneApplicationBackend(backend *svc.ApplicationBackend) *svc.ApplicationBackend {
	if backend == nil {
		return nil
	}
	cloned := *backend
	return &cloned
}

func clonePublishers(publishers []applicationapi.PublisherDeclaration) []applicationapi.PublisherDeclaration {
	if publishers == nil {
		return nil
	}
	cloned := make([]applicationapi.PublisherDeclaration, len(publishers))
	for i, publisher := range publishers {
		cloned[i] = publisher
		cloned[i].Events = cloneSlice(publisher.Events)
	}
	return cloned
}

func cloneSubscriptions(subscriptions []applicationapi.Subscription) []applicationapi.Subscription {
	if subscriptions == nil {
		return nil
	}
	cloned := make([]applicationapi.Subscription, len(subscriptions))
	for i, subscription := range subscriptions {
		cloned[i] = subscription
		cloned[i].Events = cloneSlice(subscription.Events)
	}
	return cloned
}

func cloneApplicationContainer(container *svc.ApplicationContainer) *svc.ApplicationContainer {
	if container == nil {
		return nil
	}
	cloned := *container
	cloned.Commands = cloneSlice(container.Commands)
	return &cloned
}

func cloneSlice[T any](values []T) []T {
	if values == nil {
		return nil
	}
	cloned := make([]T, len(values))
	copy(cloned, values)
	return cloned
}
