package prompt

import (
	"errors"
	"strings"
	"sync"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

var ErrNoPendingInteraction = errors.New("no active agent interaction for this chat")
var ErrInteractionQueueFull = errors.New("agent interaction response queue is full")

type interactionResponseRouter struct {
	mu     sync.Mutex
	routes map[servicechat.ID]interactionResponseRoute
}

type interactionResponseRoute struct {
	runID     uint64
	responses chan agent.InteractionResponse
}

func newInteractionResponseRouter() interactionResponseRouter {
	return interactionResponseRouter{
		routes: make(map[servicechat.ID]interactionResponseRoute),
	}
}

func (router *interactionResponseRouter) open(
	chatID servicechat.ID,
	runID uint64,
) chan agent.InteractionResponse {
	responses := make(
		chan agent.InteractionResponse,
		configconstants.PromptInteractionResponseQueueCapacity,
	)
	router.mu.Lock()
	if router.routes == nil {
		router.routes = make(map[servicechat.ID]interactionResponseRoute)
	}
	router.routes[chatID] = interactionResponseRoute{runID: runID, responses: responses}
	router.mu.Unlock()
	return responses
}

func (router *interactionResponseRouter) respond(
	chatID servicechat.ID,
	response agent.InteractionResponse,
) error {
	if strings.TrimSpace(response.ID) == "" || (len(response.Result) == 0 && len(response.Error) == 0) {
		return ErrNoPendingInteraction
	}
	router.mu.Lock()
	route, ok := router.routes[chatID]
	router.mu.Unlock()
	if !ok {
		return ErrNoPendingInteraction
	}
	select {
	case route.responses <- response:
		return nil
	default:
		return ErrInteractionQueueFull
	}
}

func (router *interactionResponseRouter) remove(chatID servicechat.ID, runID uint64) {
	router.mu.Lock()
	if route, ok := router.routes[chatID]; ok && route.runID == runID {
		delete(router.routes, chatID)
	}
	router.mu.Unlock()
}
