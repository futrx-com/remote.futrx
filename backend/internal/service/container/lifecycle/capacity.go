package lifecycle

import (
	"context"
	"fmt"
	"log"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

// containerHeadroomBytes is what a new container needs beyond its base image:
// first-boot writes and launch provisioning, plus enough left over that the
// host itself does not run dry, since the default `dir` pool shares the host's
// root filesystem with the backend, DATA_DIR and sshd.
const containerHeadroomBytes uint64 = 2 << 30

// CheckCapacity reports serviceproject.ErrInsufficientStorage when the storage
// pool cannot hold one more container from the base image. The image is
// published uncompressed, so its size is close to what unpacking it writes.
//
// It is a preflight, not a guarantee: when the pool or the image cannot be
// measured it allows the create and leaves `lxc init` to report a real failure.
func (s *Service) CheckCapacity(ctx context.Context) error {
	if !s.runtime.Available() {
		return nil
	}
	pool, free, _, err := s.runtime.StorageSpace(ctx)
	if err != nil {
		log.Printf("lifecycle: skipping capacity check: %v", err)
		return nil
	}
	imageSize, err := s.runtime.ImageSize(ctx, s.image)
	if err != nil {
		log.Printf("lifecycle: skipping capacity check: %v", err)
		return nil
	}
	need := imageSize + containerHeadroomBytes
	if free >= need {
		return nil
	}
	return fmt.Errorf(
		"%w: %s free in LXD storage pool %q, a new project needs about %s; free up space on the server and try again",
		serviceproject.ErrInsufficientStorage, formatGiB(free), pool, formatGiB(need),
	)
}

func formatGiB(bytes uint64) string {
	return fmt.Sprintf("%.1f GiB", float64(bytes)/(1<<30))
}
