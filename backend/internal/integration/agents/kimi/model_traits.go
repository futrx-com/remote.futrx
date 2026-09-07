package kimi

import "github.com/futrx-com/remote.futrx.com/internal/agent"

// modelTraits projects native feature declarations without interpreting catalog
// shape, aliases or overrides. parseModel owns those compatibility decisions.
type modelTraits struct {
	alwaysThinking bool
	canThink       bool
	modalities     []string
}

func parseModelTraits(caps []string) modelTraits {
	traits := modelTraits{modalities: []string{"text"}}
	for _, capability := range caps {
		switch capability {
		case "thinking":
			traits.canThink = true
		case "always_thinking":
			traits.canThink = true
			traits.alwaysThinking = true
		case "image_in":
			traits.modalities = append(traits.modalities, "image")
		case "video_in":
			traits.modalities = append(traits.modalities, "video")
		case "audio_in":
			traits.modalities = append(traits.modalities, "audio")
		}
	}
	return traits
}

func (t modelTraits) reasoningOptions(efforts []string) []agent.CapabilityOption {
	reasoning := []agent.CapabilityOption{}
	if len(efforts) > 0 || t.canThink {
		reasoning = append(reasoning, agent.AutoOption())
		if !t.alwaysThinking {
			reasoning = append(reasoning, agent.CapabilityOption{Value: "off", Label: "Off"})
		}
		if len(efforts) == 0 {
			efforts = []string{"on"}
		}
		for _, effort := range efforts {
			effort = agent.NormalizeCapabilityValue(effort)
			if effort != "" && !hasCapabilityOption(reasoning, effort) {
				reasoning = append(reasoning, agent.CapabilityOption{Value: effort, Label: capabilityLabel(effort)})
			}
		}
	}
	return reasoning
}
