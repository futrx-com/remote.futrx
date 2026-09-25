package applications

import (
	"strings"
	"testing"
	"testing/fstest"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	applicationapi "github.com/futrx-com/remote.futrx.com/pkg/applications"
)

func TestRegistryLoadsEventDeclarations(t *testing.T) {
	catalog := fstest.MapFS{
		"applications/event-source/application.json": {Data: []byte(`{
			"id": "event-source",
			"name": "Event Source",
			"version": "1",
			"scopes": ["project"],
			"publishers": [{
				"name": "greetings.audit",
				"events": [{
					"name": "greeting-sent",
					"version": 1,
					"description": "A greeting was recorded."
				}]
			}],
			"subscriptions": [
				{"publisher": "remote.applications", "events": ["installed", "stopped"]},
				{"publisher": "applications.audit-log.entries", "events": ["entry-created"]}
			]
		}`)},
		"applications/event-source/backend/main.go": {Data: []byte(validBackendMain)},
	}

	registry, err := NewRegistry(catalog, nil)
	if err != nil {
		t.Fatalf("load event declarations: %v", err)
	}
	application, ok := registry.Get("event-source")
	if !ok {
		t.Fatal("event source is missing")
	}
	if len(application.Publishers) != 1 || application.Publishers[0].Name != "greetings.audit" {
		t.Fatalf("publishers = %+v", application.Publishers)
	}
	if event := application.Publishers[0].Events[0]; event.Name != "greeting-sent" ||
		event.Version != 1 || event.Description != "A greeting was recorded." {
		t.Fatalf("event = %+v", event)
	}
	if len(application.Subscriptions) != 2 ||
		application.Subscriptions[0].Publisher != applicationapi.RemoteApplicationsPublisher {
		t.Fatalf("subscriptions = %+v", application.Subscriptions)
	}
}

func TestRegistryRejectsBuiltinIDOutsideCanonicalEventNamespace(t *testing.T) {
	catalog := fstest.MapFS{
		"applications/Bad_ID/application.json": {Data: []byte(`{
			"name": "Invalid event source",
			"version": "1",
			"scopes": ["project"]
		}`)},
	}

	_, err := NewRegistry(catalog, nil)
	if err == nil || !strings.Contains(err.Error(), `application id "Bad_ID"`) {
		t.Fatalf("load invalid built-in id = %v, want canonical-id error", err)
	}
}

func TestValidateAcceptsApplicationEventDeclarations(t *testing.T) {
	application := validEventApplication()
	application.Subscriptions[0].Events = []string{
		applicationapi.RemoteApplicationAdded,
		applicationapi.RemoteApplicationUpdated,
		applicationapi.RemoteApplicationDeleted,
		applicationapi.RemoteApplicationInstalled,
		applicationapi.RemoteApplicationUninstalled,
		applicationapi.RemoteApplicationStarted,
		applicationapi.RemoteApplicationStopped,
	}
	if err := validateApplication(application); err != nil {
		t.Fatalf("validate event declarations: %v", err)
	}
}

func TestValidateRejectsBadApplicationEventDeclarations(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*svc.Application)
		want   string
	}{
		{
			name: "publisher without backend",
			mutate: func(application *svc.Application) {
				application.Backend = nil
				application.Subscriptions = nil
			},
			want: "require an application backend",
		},
		{
			name: "subscription without backend",
			mutate: func(application *svc.Application) {
				application.Backend = nil
				application.Publishers = nil
			},
			want: "require an application backend",
		},
		{
			name: "canonical publisher declared as local",
			mutate: func(application *svc.Application) {
				application.Publishers[0].Name = "applications.other.events"
			},
			want: "local lowercase",
		},
		{
			name: "core publisher declared as local",
			mutate: func(application *svc.Application) {
				application.Publishers[0].Name = applicationapi.RemoteApplicationsPublisher
			},
			want: "local lowercase",
		},
		{
			name: "hyphenated core prefix declared as local",
			mutate: func(application *svc.Application) {
				application.Publishers[0].Name = "remote-events"
			},
			want: "local lowercase",
		},
		{
			name: "hyphenated applications prefix declared as local",
			mutate: func(application *svc.Application) {
				application.Publishers[0].Name = "applications-events"
			},
			want: "local lowercase",
		},
		{
			name: "invalid publisher name",
			mutate: func(application *svc.Application) {
				application.Publishers[0].Name = "Greeting_events"
			},
			want: "publisher name",
		},
		{
			name: "duplicate publisher",
			mutate: func(application *svc.Application) {
				application.Publishers = append(application.Publishers, application.Publishers[0])
			},
			want: "publisher \"greetings\" is duplicated",
		},
		{
			name: "publisher without events",
			mutate: func(application *svc.Application) {
				application.Publishers[0].Events = nil
			},
			want: "at least one event",
		},
		{
			name: "invalid published event name",
			mutate: func(application *svc.Application) {
				application.Publishers[0].Events[0].Name = "Greeting Sent"
			},
			want: "event name",
		},
		{
			name: "duplicate published event",
			mutate: func(application *svc.Application) {
				application.Publishers[0].Events = append(
					application.Publishers[0].Events, application.Publishers[0].Events[0])
			},
			want: "event \"sent\" is duplicated",
		},
		{
			name: "event without a positive version",
			mutate: func(application *svc.Application) {
				application.Publishers[0].Events[0].Version = 0
			},
			want: "version must be at least 1",
		},
		{
			name: "multiline event description",
			mutate: func(application *svc.Application) {
				application.Publishers[0].Events[0].Description = "first\nsecond"
			},
			want: "description must be one line",
		},
		{
			name: "noncanonical subscription publisher",
			mutate: func(application *svc.Application) {
				application.Subscriptions[0].Publisher = "greetings"
			},
			want: "is not canonical",
		},
		{
			name: "unknown core publisher",
			mutate: func(application *svc.Application) {
				application.Subscriptions[0].Publisher = "remote.projects"
			},
			want: "is not canonical",
		},
		{
			name: "application subscription without local publisher",
			mutate: func(application *svc.Application) {
				application.Subscriptions[0].Publisher = "applications.other"
			},
			want: "is not canonical",
		},
		{
			name: "duplicate subscription",
			mutate: func(application *svc.Application) {
				application.Subscriptions = append(application.Subscriptions, application.Subscriptions[0])
			},
			want: "subscription publisher \"remote.applications\" is duplicated",
		},
		{
			name: "subscription without events",
			mutate: func(application *svc.Application) {
				application.Subscriptions[0].Events = nil
			},
			want: "at least one event",
		},
		{
			name: "invalid subscribed event name",
			mutate: func(application *svc.Application) {
				application.Subscriptions[0].Events[0] = "App Installed"
			},
			want: "event name",
		},
		{
			name: "duplicate subscribed event",
			mutate: func(application *svc.Application) {
				application.Subscriptions[0].Events = []string{"installed", "installed"}
			},
			want: "event \"installed\" is duplicated",
		},
		{
			name: "unknown core event",
			mutate: func(application *svc.Application) {
				application.Subscriptions[0].Events[0] = "upgraded"
			},
			want: "unknown event",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			application := validEventApplication()
			test.mutate(&application)
			err := validateApplication(application)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validate = %v, want error containing %q", err, test.want)
			}
		})
	}
}

func validEventApplication() svc.Application {
	return svc.Application{
		ID:      "event-source",
		Name:    "Event Source",
		Version: "1",
		Scopes:  []svc.Scope{svc.ScopeProject},
		Backend: &svc.ApplicationBackend{},
		Publishers: []applicationapi.PublisherDeclaration{{
			Name: "greetings",
			Events: []applicationapi.EventDeclaration{{
				Name: "sent", Version: 1, Description: "A greeting was recorded.",
			}},
		}},
		Subscriptions: []applicationapi.Subscription{
			{
				Publisher: applicationapi.RemoteApplicationsPublisher,
				Events:    []string{applicationapi.RemoteApplicationInstalled},
			},
			{Publisher: "applications.audit-log.entries", Events: []string{"entry-created"}},
		},
	}
}
