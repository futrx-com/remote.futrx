package service

import (
	"context"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	serviceuser "github.com/futrx-com/remote.futrx.com/internal/service/user"
)

type projectAccessReader interface {
	ListAccess(ctx context.Context, projectID serviceproject.ID) ([]string, error)
}

type registeredUserReader interface {
	List(ctx context.Context) ([]serviceuser.User, error)
}

// chatNotificationAudience mirrors chat visibility: project chats reach
// registered members plus administrators, while loose chats reach every
// registered user.
type chatNotificationAudience struct {
	projects projectAccessReader
	users    registeredUserReader
}

func (a chatNotificationAudience) recipients(
	ctx context.Context,
	meta servicechat.Meta,
) ([]string, error) {
	users, err := a.registeredUsers(ctx)
	if err != nil {
		return nil, err
	}
	if meta.ProjectID == "" {
		return userEmails(users), nil
	}

	members, err := a.projects.ListAccess(ctx, serviceproject.ID(meta.ProjectID))
	if err != nil {
		return nil, err
	}
	active := make(map[string]serviceuser.User, len(users))
	for _, user := range users {
		active[serviceuser.NormalizeEmail(user.Email)] = user
	}
	recipients := make([]string, 0, len(members)+len(users))
	seen := make(map[string]struct{}, len(members)+len(users))
	for _, member := range members {
		email := serviceuser.NormalizeEmail(member)
		if _, registered := active[email]; !registered {
			continue
		}
		recipients = appendUniqueEmail(recipients, seen, email)
	}
	for _, user := range users {
		if user.Role == serviceuser.RoleAdmin {
			recipients = appendUniqueEmail(recipients, seen, user.Email)
		}
	}
	return recipients, nil
}

func (a chatNotificationAudience) registeredUsers(ctx context.Context) ([]serviceuser.User, error) {
	if a.users == nil {
		return nil, nil
	}
	return a.users.List(ctx)
}

func userEmails(users []serviceuser.User) []string {
	emails := make([]string, 0, len(users))
	for _, user := range users {
		emails = append(emails, serviceuser.NormalizeEmail(user.Email))
	}
	return emails
}

func appendUniqueEmail(emails []string, seen map[string]struct{}, email string) []string {
	email = serviceuser.NormalizeEmail(email)
	if email == "" {
		return emails
	}
	if _, exists := seen[email]; exists {
		return emails
	}
	seen[email] = struct{}{}
	return append(emails, email)
}
