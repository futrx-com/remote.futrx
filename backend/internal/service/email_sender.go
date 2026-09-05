package service

import (
	"context"

	"github.com/futrx-com/remote.futrx.com/internal/integration/smtp"
	serviceemail "github.com/futrx-com/remote.futrx.com/internal/service/email"
)

// emailSender adapts the Gmail SMTP integration client to the email
// service's Sender port, the only place that maps between the two type
// families and sets the wire message's From from the stored credentials.
type emailSender struct {
	client *smtp.Client
}

func (s emailSender) Verify(ctx context.Context, creds serviceemail.Credentials) error {
	return s.client.Verify(ctx, smtp.Account{Address: creds.Address, AppPassword: creds.AppPassword})
}

func (s emailSender) Send(ctx context.Context, creds serviceemail.Credentials, msg serviceemail.Message) error {
	return s.client.Send(ctx, smtp.Account{Address: creds.Address, AppPassword: creds.AppPassword}, smtp.Message{
		From:     creds.Address,
		To:       msg.To,
		Subject:  msg.Subject,
		Body:     msg.Body,
		HTMLBody: msg.HTMLBody,
	})
}
