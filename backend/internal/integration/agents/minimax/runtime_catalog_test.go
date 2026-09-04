package minimax

import (
	"encoding/json"
	"testing"

	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
)

func TestRuntimeCatalogContainsEveryDiscoveredModel(t *testing.T) {
	content, err := buildRuntimeModelCatalog([]string{"MiniMax-M3", "MiniMax-M2.7"})
	if err != nil {
		t.Fatal(err)
	}
	var catalog runtimeModelCatalog
	if err := json.Unmarshal(content, &catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Models) != 2 || catalog.Models[0].Slug != "MiniMax-M3" ||
		catalog.Models[1].Slug != "MiniMax-M2.7" || catalog.Models[1].Priority != 1 {
		t.Fatalf("catalog = %#v", catalog.Models)
	}
	if len(catalog.Models[0].SupportedReasoningLevels) != 2 ||
		catalog.Models[0].SupportedReasoningLevels[0].Effort != "none" {
		t.Fatalf("M3 reasoning = %#v", catalog.Models[0].SupportedReasoningLevels)
	}
	if len(catalog.Models[1].SupportedReasoningLevels) != 1 ||
		catalog.Models[1].SupportedReasoningLevels[0].Effort != "high" {
		t.Fatalf("M2.7 reasoning = %#v", catalog.Models[1].SupportedReasoningLevels)
	}

	asset := miniMaxRuntimeCatalogAsset(content)
	if asset.Path != configconstants.MiniMaxContainerCatalog ||
		asset.HashPath != configconstants.MiniMaxContainerCatalogHash {
		t.Fatalf("asset = %#v", asset)
	}
}

func TestRuntimeCatalogRejectsNonLanguageModels(t *testing.T) {
	for _, models := range [][]string{nil, {"image-01"}, {"MiniMax-M3 unsafe"}} {
		if _, err := buildRuntimeModelCatalog(models); err == nil {
			t.Fatalf("models %#v unexpectedly accepted", models)
		}
	}
}
