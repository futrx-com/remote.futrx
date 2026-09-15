package service

import "context"

// ChatLifecyclePublisher is the notification capability used by the
// persistence decorator after chat record mutations succeed.
type ChatLifecyclePublisher interface {
	PublishChatCreated(context.Context, string)
	PublishChatUpdated(context.Context, string)
	PublishChatDeleted(context.Context, string)
}

// ProjectLifecyclePublisher is the notification capability used by the
// persistence decorator after project record mutations succeed.
type ProjectLifecyclePublisher interface {
	PublishProjectCreated(context.Context, string)
	PublishProjectUpdated(context.Context, string)
	PublishProjectDeleted(context.Context, string)
}
