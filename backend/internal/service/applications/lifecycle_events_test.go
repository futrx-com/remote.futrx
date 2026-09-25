package applications

import (
	"context"
	"errors"
	"testing"
)

type recordedApplicationLifecycleEvent struct {
	kind          string
	applicationID string
	instanceID    string
	scope         string
	projectID     string
	ctx           context.Context
}

type recordingApplicationLifecyclePublisher struct {
	events []recordedApplicationLifecycleEvent
}

func (p *recordingApplicationLifecyclePublisher) catalog(
	ctx context.Context,
	kind, applicationID string,
) {
	p.events = append(p.events, recordedApplicationLifecycleEvent{
		kind: kind, applicationID: applicationID, ctx: ctx,
	})
}

func (p *recordingApplicationLifecyclePublisher) instance(
	ctx context.Context,
	kind, applicationID, instanceID, scope, projectID string,
) {
	p.events = append(p.events, recordedApplicationLifecycleEvent{
		kind:          kind,
		applicationID: applicationID,
		instanceID:    instanceID,
		scope:         scope,
		projectID:     projectID,
		ctx:           ctx,
	})
}

func (p *recordingApplicationLifecyclePublisher) PublishApplicationAdded(
	ctx context.Context,
	applicationID string,
) {
	p.catalog(ctx, "added", applicationID)
}

func (p *recordingApplicationLifecyclePublisher) PublishApplicationUpdated(
	ctx context.Context,
	applicationID string,
) {
	p.catalog(ctx, "updated", applicationID)
}

func (p *recordingApplicationLifecyclePublisher) PublishApplicationDeleted(
	ctx context.Context,
	applicationID string,
) {
	p.catalog(ctx, "deleted", applicationID)
}

func (p *recordingApplicationLifecyclePublisher) PublishApplicationInstalled(
	ctx context.Context,
	applicationID, instanceID, scope, projectID string,
) {
	p.instance(ctx, "installed", applicationID, instanceID, scope, projectID)
}

func (p *recordingApplicationLifecyclePublisher) PublishApplicationUninstalled(
	ctx context.Context,
	applicationID, instanceID, scope, projectID string,
) {
	p.instance(ctx, "uninstalled", applicationID, instanceID, scope, projectID)
}

func (p *recordingApplicationLifecyclePublisher) PublishApplicationStarted(
	ctx context.Context,
	applicationID, instanceID, scope, projectID string,
) {
	p.instance(ctx, "started", applicationID, instanceID, scope, projectID)
}

func (p *recordingApplicationLifecyclePublisher) PublishApplicationStopped(
	ctx context.Context,
	applicationID, instanceID, scope, projectID string,
) {
	p.instance(ctx, "stopped", applicationID, instanceID, scope, projectID)
}

func TestApplicationInstanceLifecyclePublishesCommittedTransitions(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		scope     Scope
		projectID string
	}{
		{name: "global", scope: ScopeGlobal},
		{name: "project", scope: ScopeProject, projectID: "project-1"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			application := Application{
				ID: "demo", Name: "Demo", Version: "1",
				Scopes: []Scope{ScopeGlobal, ScopeProject},
				UI:     &ApplicationUI{},
			}
			store := &fakeStore{}
			publisher := &recordingApplicationLifecyclePublisher{}
			service := New(
				&singleApplicationRegistry{application: application},
				store, nil, nil, nil,
				WithLifecyclePublisher(publisher),
			)
			ctx := context.Background()

			installed, err := service.Install(ctx, InstallRequest{
				ApplicationID: application.ID,
				Scope:         testCase.scope,
				ProjectID:     testCase.projectID,
			})
			if err != nil {
				t.Fatalf("install: %v", err)
			}
			// Same-state commands may reconverge runtime state, but they are not
			// another lifecycle transition and must not publish duplicates.
			if _, err := service.Start(ctx, installed.ID); err != nil {
				t.Fatalf("same-state start: %v", err)
			}
			if _, err := service.Stop(ctx, installed.ID); err != nil {
				t.Fatalf("stop: %v", err)
			}
			if _, err := service.Stop(ctx, installed.ID); err != nil {
				t.Fatalf("same-state stop: %v", err)
			}
			if _, err := service.Start(ctx, installed.ID); err != nil {
				t.Fatalf("start: %v", err)
			}
			if err := service.Uninstall(ctx, installed.ID); err != nil {
				t.Fatalf("uninstall: %v", err)
			}

			wantKinds := []string{"installed", "stopped", "started", "uninstalled"}
			if len(publisher.events) != len(wantKinds) {
				t.Fatalf("events = %+v, want kinds %v", publisher.events, wantKinds)
			}
			for index, kind := range wantKinds {
				event := publisher.events[index]
				if event.kind != kind || event.applicationID != "demo" ||
					event.instanceID != installed.ID || event.scope != string(testCase.scope) ||
					event.projectID != testCase.projectID {
					t.Fatalf("event %d = %+v", index, event)
				}
				if event.ctx != ctx {
					t.Fatalf("event %d did not retain the operation context", index)
				}
			}
		})
	}
}

func TestApplicationLifecycleDoesNotPublishFailedOperations(t *testing.T) {
	application := Application{
		ID: "demo", Name: "Demo", Version: "1", Scopes: []Scope{ScopeGlobal}, UI: &ApplicationUI{},
	}
	publisher := &recordingApplicationLifecyclePublisher{}
	store := &fakeStore{
		global: []Instance{{
			ID: "instance-1", ApplicationID: "demo", Scope: ScopeGlobal, Status: StatusRunning,
		}},
		deleteErr: errors.New("disk full"),
	}
	service := New(
		&singleApplicationRegistry{application: application},
		store, nil, nil, nil,
		WithLifecyclePublisher(publisher),
	)

	if err := service.Uninstall(context.Background(), "instance-1"); err == nil {
		t.Fatal("uninstall succeeded despite the store failure")
	}
	if len(publisher.events) != 0 {
		t.Fatalf("failed uninstall published events: %+v", publisher.events)
	}
}

func TestApplicationCatalogLifecycleDistinguishesAddUpdateAndDelete(t *testing.T) {
	publisher := &recordingApplicationLifecyclePublisher{}
	catalog := &recordingCatalog{}
	service := New(
		&fakeRegistry{}, &fakeStore{}, nil, nil, nil,
		WithPackageCatalog(catalog),
		WithLifecyclePublisher(publisher),
	)
	ctx := context.Background()

	for range 2 {
		if _, err := service.UploadPackage(ctx, PackageUpload{Data: []byte("PK")}); err != nil {
			t.Fatalf("upload: %v", err)
		}
	}
	if _, err := service.RemovePackage(ctx, RemovePackageRequest{ID: "uploaded-app"}); err != nil {
		t.Fatalf("remove: %v", err)
	}

	wantKinds := []string{"added", "updated", "deleted"}
	if len(publisher.events) != len(wantKinds) {
		t.Fatalf("events = %+v, want kinds %v", publisher.events, wantKinds)
	}
	for index, kind := range wantKinds {
		if event := publisher.events[index]; event.kind != kind || event.applicationID != "uploaded-app" {
			t.Fatalf("event %d = %+v", index, event)
		}
	}
}

func TestApplicationAddedPublishesEvenWhenUpgradeEnumerationFails(t *testing.T) {
	publisher := &recordingApplicationLifecyclePublisher{}
	service := New(
		&fakeRegistry{},
		&fakeStore{listAllErr: errors.New("cannot enumerate instances")},
		nil, nil, nil,
		WithPackageCatalog(&recordingCatalog{}),
		WithLifecyclePublisher(publisher),
	)

	if _, err := service.UploadPackage(context.Background(), PackageUpload{Data: []byte("PK")}); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if len(publisher.events) != 1 || publisher.events[0].kind != "added" {
		t.Fatalf("events = %+v, want one added event", publisher.events)
	}
}

func TestPackageRemovalPublishesUninstallsBeforeDelete(t *testing.T) {
	publisher := &recordingApplicationLifecyclePublisher{}
	store := &fakeStore{
		global: []Instance{instance("uploaded-app", "", StatusRunning)},
		byProject: map[string][]Instance{
			"project-1": {instance("uploaded-app", "project-1", StatusStopped)},
		},
	}
	catalog := &recordingCatalog{stored: []PackageView{{
		Package: Package{ID: "uploaded-app", Name: "Uploaded App"},
	}}}
	service := New(
		&fakeRegistry{}, store, nil, nil, nil,
		WithPackageCatalog(catalog),
		WithLifecyclePublisher(publisher),
	)

	if _, err := service.RemovePackage(context.Background(), RemovePackageRequest{
		ID: "uploaded-app", UninstallInstalled: true,
	}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(publisher.events) != 3 {
		t.Fatalf("events = %+v, want two uninstalls then delete", publisher.events)
	}
	for index := 0; index < 2; index++ {
		if publisher.events[index].kind != "uninstalled" {
			t.Fatalf("event %d = %+v, want uninstall", index, publisher.events[index])
		}
	}
	if publisher.events[2].kind != "deleted" {
		t.Fatalf("last event = %+v, want delete", publisher.events[2])
	}
}
