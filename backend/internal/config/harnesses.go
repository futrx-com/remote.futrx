package config

// This file is the composition root for Remote's compiled-in harness
// integrations. Providers own their complete factory; config owns only the
// reviewed registration order and has no provider construction details.

import (
	antigravityharness "github.com/futrx-com/remote.futrx.com/internal/integration/harnesses/antigravity"
	claudeharness "github.com/futrx-com/remote.futrx.com/internal/integration/harnesses/claude"
	codexharness "github.com/futrx-com/remote.futrx.com/internal/integration/harnesses/codex"
	kimiharness "github.com/futrx-com/remote.futrx.com/internal/integration/harnesses/kimi"
	minimaxharness "github.com/futrx-com/remote.futrx.com/internal/integration/harnesses/minimax"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
)

func NewHarnessModules() (*agentmodule.Catalog, error) {
	builders := []agentmodule.FactoryBuilder{
		claudeharness.NewFactory,
		codexharness.NewFactory,
		minimaxharness.NewFactory,
		kimiharness.NewFactory,
		antigravityharness.NewFactory,
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
