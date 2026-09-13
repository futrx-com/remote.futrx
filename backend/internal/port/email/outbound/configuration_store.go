// Package emailoutbound holds the email service's outbound contracts:
// capabilities the service consumes, implemented by an integration or store
// adapter at composition. The service depends on these interfaces, never on
// a concrete transport or persistence type.
package emailoutbound

import (
	"context"

	emailapplication "github.com/futrx-com/remote.futrx.com/internal/model/email/application"
)

// ConfigurationStore persists the single server-wide SMTP configuration.
// Configuration returns (nil, nil) when nothing has ever been saved - that
// absence is the correct "not configured" state, not an error, mirroring
// service/auth.TwoFactorStore.Get.
type ConfigurationStore interface {
	Configuration(ctx context.Context) (*emailapplication.SMTPConfiguration, error)
	Save(ctx context.Context, cfg emailapplication.SMTPConfiguration) error
	Delete(ctx context.Context) error
}
