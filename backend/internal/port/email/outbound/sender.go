package emailoutbound

import (
	"context"

	emailapplication "github.com/futrx-com/remote.futrx.com/internal/model/email/application"
	emaildomain "github.com/futrx-com/remote.futrx.com/internal/model/email/domain"
)

// Sender speaks the outbound SMTP protocol against a validated configuration.
// The composition layer supplies the real SMTP client; tests supply a
// recorder.
type Sender interface {
	Verify(ctx context.Context, cfg emailapplication.SMTPConfiguration) error
	Send(ctx context.Context, cfg emailapplication.SMTPConfiguration, msg emaildomain.Message) error
}
