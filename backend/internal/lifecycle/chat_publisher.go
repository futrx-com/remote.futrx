package lifecycle

import "context"

// ChatState identifies a persisted chat lifecycle transition.
type ChatState string

const (
	ChatCreated ChatState = "created"
	ChatUpdated ChatState = "updated"
	ChatDeleted ChatState = "deleted"
)

// ChatEvent identifies a persisted chat lifecycle transition without exposing
// transcript content, prompts, agent output, or provider session identifiers.
type ChatEvent struct {
	State  ChatState
	ChatID string
}

// ChatSubscriber receives chat lifecycle events.
type ChatSubscriber interface {
	OnChat(context.Context, ChatEvent)
}

// ChatPublisher owns chat lifecycle subscribers and dispatches events to them
// synchronously in registration order.
type ChatPublisher struct {
	events eventDispatcher[ChatEvent]
}

// NewChatPublisher creates a publisher with no subscribers.
func NewChatPublisher() *ChatPublisher {
	return &ChatPublisher{}
}

// Subscribe registers a subscriber and returns an idempotent function that
// removes it.
func (p *ChatPublisher) Subscribe(subscriber ChatSubscriber) (unsubscribe func()) {
	return p.events.subscribe(func(ctx context.Context, event ChatEvent) {
		subscriber.OnChat(ctx, event)
	})
}

// PublishChatCreated reports that a chat record was created.
func (p *ChatPublisher) PublishChatCreated(ctx context.Context, chatID string) {
	p.publish(ctx, ChatCreated, chatID)
}

// PublishChatUpdated reports that a chat record was updated.
func (p *ChatPublisher) PublishChatUpdated(ctx context.Context, chatID string) {
	p.publish(ctx, ChatUpdated, chatID)
}

// PublishChatDeleted reports that a chat record was deleted.
func (p *ChatPublisher) PublishChatDeleted(ctx context.Context, chatID string) {
	p.publish(ctx, ChatDeleted, chatID)
}

func (p *ChatPublisher) publish(ctx context.Context, state ChatState, chatID string) {
	p.events.publish(ctx, ChatEvent{State: state, ChatID: chatID})
}
