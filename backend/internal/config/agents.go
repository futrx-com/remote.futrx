package config

// This file is the composition root for Remote's compiled-in agent
// integrations. Providers own their complete factory; config owns only the
// reviewed registration order and has no provider construction details.

import (
	antigravityagent "github.com/futrx-com/remote.futrx.com/internal/integration/agents/antigravity"
	claudeagent "github.com/futrx-com/remote.futrx.com/internal/integration/agents/claude"
	codexagent "github.com/futrx-com/remote.futrx.com/internal/integration/agents/codex"
	kimiagent "github.com/futrx-com/remote.futrx.com/internal/integration/agents/kimi"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
)

func NewAgentModules() (*agentmodule.Catalog, error) {
	builders := []agentmodule.FactoryBuilder{
		claudeagent.NewFactory,
		codexagent.NewFactory,
		kimiagent.NewFactory,
		antigravityagent.NewFactory,
	}
	factories := make([]agentmodule.Factory, 0, len(builders))
	for _, build := range builders {
		factory, err := build()
		if err != nil {
			return nil, err
		}
		factories = append(factories, factory)
	}
	return agentmodule.NewCatalog(factories...)
}
