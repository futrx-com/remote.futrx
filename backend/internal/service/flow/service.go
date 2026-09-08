package flow

import (
	"context"
	"fmt"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

type ChatRepository interface {
	EventPage(ctx context.Context, id servicechat.ID, query servicechat.EventPageQuery) (servicechat.EventPage, error)
}

type Service struct {
	chats  ChatRepository
	mapper *FlowMapper
}

func NewService(chats ChatRepository) *Service {
	return &Service{
		chats:  chats,
		mapper: NewFlowMapper(),
	}
}

func (s *Service) GetMapState(ctx context.Context, chatID string) (FlowMapState, error) {
	if s.chats == nil {
		return FlowMapState{ChatID: chatID}, nil
	}

	page, err := s.chats.EventPage(ctx, servicechat.ID(chatID), servicechat.EventPageQuery{
		Limit: 1000,
	})
	if err != nil {
		return FlowMapState{}, fmt.Errorf("fetch chat events for flow map: %w", err)
	}

	return s.mapper.BuildState(chatID, page.Events), nil
}

func (s *Service) ProcessEvents(chatID string, events []servicechat.Event) FlowMapState {
	return s.mapper.BuildState(chatID, events)
}
