package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

func TestCheckCapacityComparesFreeSpaceWithTheImagePlusHeadroom(t *testing.T) {
	const image = 3 << 30
	tests := []struct {
		name    string
		runtime recordingRuntime
		wantErr bool
		wantMsg string
	}{
		{name: "enough", runtime: recordingRuntime{available: true, free: image + containerHeadroomBytes, imageSize: image}},
		{
			name:    "short",
			runtime: recordingRuntime{available: true, free: 1 << 30, imageSize: image},
			wantErr: true,
			wantMsg: `1.0 GiB free in LXD storage pool "default", a new project needs about 5.0 GiB`,
		},
		{name: "pool unmeasurable", runtime: recordingRuntime{available: true, spaceErr: errors.New("no pool")}},
		{name: "image unmeasurable", runtime: recordingRuntime{available: true, free: 1, imageErr: errors.New("no alias")}},
		{name: "lxc unavailable", runtime: recordingRuntime{available: false}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var events []string
			runtime := test.runtime
			runtime.events = &events

			err := newTestService(&runtime, &events).CheckCapacity(context.Background())
			if !test.wantErr {
				if err != nil {
					t.Fatalf("CheckCapacity() error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, serviceproject.ErrInsufficientStorage) {
				t.Fatalf("CheckCapacity() error = %v, want ErrInsufficientStorage", err)
			}
			if !strings.Contains(err.Error(), test.wantMsg) {
				t.Fatalf("CheckCapacity() error = %q, want it to contain %q", err, test.wantMsg)
			}
		})
	}
}

func TestCheckCapacityMeasuresTheConfiguredImage(t *testing.T) {
	var events []string
	runtime := &recordingRuntime{events: &events, available: true, free: 100 << 30, imageSize: 1 << 30}

	if err := newTestService(runtime, &events).CheckCapacity(context.Background()); err != nil {
		t.Fatalf("CheckCapacity() error = %v", err)
	}
	if !strings.Contains(strings.Join(events, "\n"), "runtime image size local:remote-base") {
		t.Fatalf("events = %q, want the service image measured", events)
	}
}
