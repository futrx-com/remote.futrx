package config

// This file is the composition root for Remote's compiled-in agent
// integrations. Providers own their complete factory; config owns only the
// reviewed registration order and has no provider construction details.

import (
	antigravityharness "github.com/futrx-com/remote.futrx.com/internal/integration/harnesses/antigravity"
	claudeharness "github.com/futrx-com/remote.futrx.com/internal/integration/harnesses/claude"
	codexharness "github.com/futrx-com/remote.futrx.com/internal/integration/harnesses/codex"
	kimiharness "github.com/futrx-com/remote.futrx.com/internal/integration/harnesses/kimi"
	minimaxharness "github.com/futrx-com/remote.futrx.com/internal/integration/harnesses/minimax"
	harnessmodule "github.com/futrx-com/remote.futrx.com/internal/service/harness/module"
)

func NewHarnessModules() (*harnessmodule.Catalog, error) {
	builders := []harnessmodule.FactoryBuilder{
		claudeharness.NewFactory,
		codexharness.NewFactory,
		minimaxharness.NewFactory,
		kimiharness.NewFactory,
		antigravityharness.NewFactory,
	}
	factories := make([]harnessmodule.Factory, 0, len(builders))
	for _, build := range builders {
		factory, err := build()
		if err != nil {
			return nil, err
		}
		factories = append(factories, factory)
	}
	return harnessmodule.NewCatalog(factories...)
}
