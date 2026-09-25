package applications

import (
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	applicationapi "github.com/futrx-com/remote.futrx.com/pkg/applications"
)

func TestDecodeApplicationManifestRequiresExactUniqueFields(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "unknown top-level field",
			raw:  `{"name":"Example","mystery":true}`,
			want: `unknown JSON field "mystery" at $`,
		},
		{
			name: "case alias at top level",
			raw:  `{"name":"Example","Publishers":[]}`,
			want: `unknown JSON field "Publishers" at $`,
		},
		{
			name: "case alias in publisher",
			raw:  `{"publishers":[{"Name":"greetings","events":[]}]}`,
			want: `unknown JSON field "Name" at $.publishers[0]`,
		},
		{
			name: "unknown nested event field",
			raw:  `{"publishers":[{"name":"greetings","events":[{"name":"sent","version":1,"schema":{}}]}]}`,
			want: `unknown JSON field "schema" at $.publishers[0].events[0]`,
		},
		{
			name: "duplicate top-level field",
			raw:  `{"name":"First","name":"Second"}`,
			want: `duplicate JSON field "name" at $`,
		},
		{
			name: "duplicate nested field",
			raw:  `{"publishers":[{"name":"first","name":"second","events":[]}]}`,
			want: `duplicate JSON field "name" at $.publishers[0]`,
		},
		{
			name: "derived source field",
			raw:  `{"name":"Example","source":"builtin"}`,
			want: `unknown JSON field "source" at $`,
		},
		{
			name: "multiple values",
			raw:  `{} {}`,
			want: "exactly one JSON value",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var application svc.Application
			err := decodeApplicationManifest([]byte(test.raw), &application)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("decode = %v, want error containing %q", err, test.want)
			}
		})
	}
}

func TestDecodeApplicationManifestAcceptsDynamicMapKeys(t *testing.T) {
	raw := []byte(`{
		"name":"Example",
		"hostTools":[{
			"name":"example",
			"version":"1",
			"downloads":{"custom-architecture":{"url":"https://example.invalid/tool","sha256":"abc"}}
		}],
		"ui":{"views":{"custom-view":"views/custom.html"}}
	}`)
	var application svc.Application
	if err := decodeApplicationManifest(raw, &application); err != nil {
		t.Fatalf("decode manifest with map keys: %v", err)
	}
}

func TestDecodeApplicationManifestRejectsOversizeInput(t *testing.T) {
	raw := append([]byte(`{}`), []byte(strings.Repeat(" ", maxApplicationManifestBytes))...)
	var application svc.Application
	err := decodeApplicationManifest(raw, &application)
	if err == nil || !strings.Contains(err.Error(), "larger than 256 KiB") {
		t.Fatalf("decode = %v, want manifest size error", err)
	}
}

func TestPersistedUploadKeepsLegacyManifestJSONCompatibility(t *testing.T) {
	legacyPadding := strings.Repeat("x", maxApplicationManifestBytes)
	catalog := fstest.MapFS{
		"applications/legacy/application.json": &fstest.MapFile{Data: []byte(`{
			"ID":"legacy",
			"name":"Legacy",
			"version":"1",
			"scopes":["global"],
			"source":"builtin",
			"container":{"commands":["claimed"],"sourceDigest":"claimed","buildVersion":"claimed"},
			"skills":["claimed"],
			"futureField":{"ignored":true},
			"publishers":"Acme Publishing",
			"subscriptions":{"note":"not an event contract"},
			"legacyPadding":"` + legacyPadding + `",
			"ui":{"entry":"main.js"}
		}`)},
		"applications/legacy/ui/main.js": &fstest.MapFile{Data: []byte("export default function () {}\n")},
	}

	if _, _, err := loadApplication(catalog, "legacy"); err == nil {
		t.Fatal("strict new-package load accepted legacy manifest fields")
	}
	view := newCatalogView()
	skipped, err := loadCatalogInto(
		&view,
		catalog,
		svc.SourceUploaded,
		func(string) error { return nil },
	)
	if err != nil {
		t.Fatalf("load persisted catalog: %v", err)
	}
	if len(skipped) != 0 {
		t.Fatalf("persisted package was skipped: %v", skipped)
	}
	application, ok := view.byID["legacy"]
	if !ok {
		t.Fatal("persisted package is missing from the live catalog")
	}
	if application.Source != svc.SourceUploaded {
		t.Fatalf("source = %q, want host-derived uploaded", application.Source)
	}
	if application.Container != nil || len(application.Skills) != 0 {
		t.Fatalf(
			"manifest-derived fields were trusted: container=%+v skills=%v",
			application.Container,
			application.Skills,
		)
	}
	if len(application.Publishers) != 0 || len(application.Subscriptions) != 0 {
		t.Fatalf(
			"legacy unknown fields activated event capabilities: publishers=%v subscriptions=%v",
			application.Publishers,
			application.Subscriptions,
		)
	}
}

func TestPersistedUploadRetainsStrictCurrentEventManifest(t *testing.T) {
	raw := []byte(`{
		"id":"current",
		"name":"Current",
		"version":"1",
		"scopes":["global"],
		"publishers":[{"name":"jobs","events":[{"name":"completed","version":1}]}],
		"subscriptions":[{"publisher":"remote.applications","events":["installed"]}]
	}`)
	var application svc.Application
	if err := decodePersistedApplicationManifest(raw, &application); err != nil {
		t.Fatalf("decode current persisted manifest: %v", err)
	}
	if len(application.Publishers) != 1 || application.Publishers[0].Name != "jobs" {
		t.Fatalf("publishers = %+v, want current strict declaration", application.Publishers)
	}
	if len(application.Subscriptions) != 1 ||
		application.Subscriptions[0].Publisher != applicationapi.RemoteApplicationsPublisher {
		t.Fatalf("subscriptions = %+v, want current strict declaration", application.Subscriptions)
	}
}

func TestValidateRejectsOversizeApplicationEventSchema(t *testing.T) {
	base := func() svc.Application {
		return svc.Application{
			Name:    "Events",
			Version: "1",
			Scopes:  []svc.Scope{svc.ScopeProject},
			Backend: &svc.ApplicationBackend{},
			Publishers: []applicationapi.PublisherDeclaration{{
				Name: "greetings",
				Events: []applicationapi.EventDeclaration{{
					Name: "sent", Version: 1, Description: "A greeting was sent.",
				}},
			}},
			Subscriptions: []applicationapi.Subscription{{
				Publisher: applicationapi.RemoteApplicationsPublisher,
				Events:    []string{"installed"},
			}},
		}
	}

	for _, test := range []struct {
		name   string
		mutate func(*svc.Application)
		want   string
	}{
		{
			name: "publisher count",
			mutate: func(application *svc.Application) {
				application.Publishers = make([]applicationapi.PublisherDeclaration, maxApplicationPublishers+1)
			},
			want: "publishers must not contain more than",
		},
		{
			name: "publisher name length",
			mutate: func(application *svc.Application) {
				application.Publishers[0].Name = strings.Repeat("a", maxLocalPublisherNameBytes+1)
			},
			want: "publisher name must not exceed",
		},
		{
			name: "published event count",
			mutate: func(application *svc.Application) {
				application.Publishers[0].Events = make([]applicationapi.EventDeclaration, maxPublisherEvents+1)
			},
			want: "must not declare more than",
		},
		{
			name: "published event name length",
			mutate: func(application *svc.Application) {
				application.Publishers[0].Events[0].Name = strings.Repeat("a", maxEventNameBytes+1)
			},
			want: "event name must not exceed",
		},
		{
			name: "published event description length",
			mutate: func(application *svc.Application) {
				application.Publishers[0].Events[0].Description = strings.Repeat("a", maxEventDescriptionBytes+1)
			},
			want: "description must not exceed",
		},
		{
			name: "subscription count",
			mutate: func(application *svc.Application) {
				application.Subscriptions = make([]applicationapi.Subscription, maxApplicationSubscriptions+1)
			},
			want: "subscriptions must not contain more than",
		},
		{
			name: "subscription publisher length",
			mutate: func(application *svc.Application) {
				application.Subscriptions[0].Publisher = strings.Repeat("a", maxCanonicalPublisherNameBytes+1)
			},
			want: "subscription publisher must not exceed",
		},
		{
			name: "subscribed event count",
			mutate: func(application *svc.Application) {
				application.Subscriptions[0].Events = make([]string, maxSubscriptionEvents+1)
			},
			want: "must not declare more than",
		},
		{
			name: "subscribed event name length",
			mutate: func(application *svc.Application) {
				application.Subscriptions[0].Events[0] = strings.Repeat("a", maxEventNameBytes+1)
			},
			want: "event name must not exceed",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			application := base()
			test.mutate(&application)
			err := validateApplication(application)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validate = %v, want error containing %q", err, test.want)
			}
		})
	}
}

func TestRegistryListAndGetDeepCloneApplications(t *testing.T) {
	original := svc.Application{
		ID:      "cloned",
		Name:    "Cloned",
		Version: "1",
		HostTools: []svc.HostTool{{
			Name: "tool", VersionArgs: []string{"version"},
			Downloads: map[string]svc.HostToolDownload{
				"amd64": {URL: "https://example.invalid/tool"},
			},
		}},
		Scopes: []svc.Scope{svc.ScopeProject},
		Env:    []svc.EnvVar{{Key: "GREETING"}},
		Service: &svc.ApplicationService{
			Name:        "cloned",
			Command:     []string{"/usr/local/bin/cloned", "serve"},
			Environment: []svc.ServiceEnvironment{{Key: "GREETING_B64", FromEnv: "GREETING"}},
		},
		UI: &svc.ApplicationUI{
			Styles: []string{"styles/main.css"},
			Views:  map[string]string{"panel": "views/panel.html"},
		},
		Backend: &svc.ApplicationBackend{TimeoutMS: 1234},
		Publishers: []applicationapi.PublisherDeclaration{{
			Name: "greetings",
			Events: []applicationapi.EventDeclaration{{
				Name: "sent", Version: 1,
			}},
		}},
		Subscriptions: []applicationapi.Subscription{{
			Publisher: applicationapi.RemoteApplicationsPublisher,
			Events:    []string{"installed"},
		}},
		Container: &svc.ApplicationContainer{Commands: []string{"server"}},
		Skills:    []string{"inspector"},
	}
	registry := &Registry{view: catalogView{
		byID:   map[string]svc.Application{original.ID: original},
		sorted: []svc.Application{original},
	}}

	listed := registry.List()[0]
	mutateEveryApplicationReference(&listed)
	got, ok := registry.Get(original.ID)
	if !ok {
		t.Fatal("Get did not find application")
	}
	if !reflect.DeepEqual(got, original) {
		t.Fatalf("List result mutated registry:\n got: %#v\nwant: %#v", got, original)
	}

	mutateEveryApplicationReference(&got)
	fresh := registry.List()[0]
	if !reflect.DeepEqual(fresh, original) {
		t.Fatalf("Get result mutated registry:\n got: %#v\nwant: %#v", fresh, original)
	}
}

func mutateEveryApplicationReference(application *svc.Application) {
	application.HostTools[0].Name = "changed"
	application.HostTools[0].VersionArgs[0] = "changed"
	application.HostTools[0].Downloads["amd64"] = svc.HostToolDownload{URL: "changed"}
	application.Scopes[0] = svc.ScopeGlobal
	application.Env[0].Key = "CHANGED"
	application.Service.Name = "changed"
	application.Service.Command[0] = "changed"
	application.Service.Environment[0].Key = "CHANGED"
	application.UI.Styles[0] = "changed"
	application.UI.Views["panel"] = "changed"
	application.Backend.TimeoutMS = 9999
	application.Publishers[0].Name = "changed"
	application.Publishers[0].Events[0].Name = "changed"
	application.Subscriptions[0].Publisher = "changed"
	application.Subscriptions[0].Events[0] = "changed"
	application.Container.Commands[0] = "changed"
	application.Skills[0] = "changed"
}
