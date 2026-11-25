package chat

import (
	"errors"
	"slices"

	"github.com/futrx-com/remote.futrx.com/internal/service/project"
)

var workspaceFacets = [...]string{"project", "provider", "model", "mode", "status", "effort", "tier", "skill"}

var ErrInvalidWorkspaceSearch = errors.New("invalid workspace search selection")

func (selection WorkspaceSearchSelection) Validate() error {
	switch selection.DateField {
	case "", "lastMessageAt", "createdAt":
	default:
		return ErrInvalidWorkspaceSearch
	}
	for id := range selection.Facets {
		if !slices.Contains(workspaceFacets[:], id) {
			return ErrInvalidWorkspaceSearch
		}
	}
	return nil
}

func workspaceProjectsByID(projects []project.Meta) map[ProjectID]*project.Meta {
	index := make(map[ProjectID]*project.Meta, len(projects))
	for i := range projects {
		index[ProjectID(projects[i].ID)] = &projects[i]
	}
	return index
}

func (s WorkspaceSearchSelection) includesDate(meta Meta) bool {
	at := meta.LastMessageAt
	if s.DateField == "createdAt" {
		at = meta.CreatedAt
	}
	return (s.From == nil || at >= *s.From) && (s.To == nil || at <= *s.To)
}

type workspaceChatFacets [len(workspaceFacets)][]string

func workspaceFacetValues(meta Meta, p *project.Meta, skills []string) workspaceChatFacets {
	projectID := " unassigned"
	if p != nil {
		projectID = string(p.ID)
	}
	var status []string
	if meta.LastMessageAt > meta.LastReadAt {
		status = append(status, "unread")
	}
	if meta.Running {
		status = append(status, "running")
	}

	return workspaceChatFacets{
		{projectID}, {string(meta.Provider)}, {meta.Model}, {meta.Mode}, status,
		{meta.ReasoningEffort}, {meta.ServiceTier}, skills,
	}
}

func workspaceSkillNames(meta Meta) []string {
	names := make([]string, 0, len(meta.SelectedSkills))
	for _, skill := range meta.SelectedSkills {
		names = append(names, skill.Name)
	}
	return names
}

const allWorkspaceFacets = (1 << len(workspaceFacets)) - 1

func (values workspaceChatFacets) matchMask(selection WorkspaceSearchSelection) int {
	mask := 0
	for i, id := range workspaceFacets {
		selected := selection.Facets[id]
		if len(selected) == 0 || slices.ContainsFunc(values[i], func(value string) bool {
			return slices.Contains(selected, value)
		}) {
			mask |= 1 << i
		}
	}
	return mask
}

// WorkspaceMessageChatIDs applies chat facets and dates to the authorized
// chats. The message query itself is evaluated only against transcripts.
func WorkspaceMessageChatIDs(metas []Meta, projects []project.Meta, selection WorkspaceSearchSelection) []ID {
	index := workspaceProjectsByID(projects)
	ids := make([]ID, 0, len(metas))
	for _, meta := range metas {
		if !selection.includesDate(meta) {
			continue
		}
		values := workspaceFacetValues(meta, index[meta.ProjectID], workspaceSkillNames(meta))
		if values.matchMask(selection) == allWorkspaceFacets {
			ids = append(ids, meta.ID)
		}
	}
	return ids
}

func newWorkspaceFacetCounts() map[string]map[string]int {
	counts := make(map[string]map[string]int, len(workspaceFacets))
	for _, id := range workspaceFacets {
		counts[id] = make(map[string]int)
	}
	return counts
}

func (values workspaceChatFacets) countMatches(counts map[string]map[string]int, mask int) {
	for i, id := range workspaceFacets {
		// A facet's counts answer how many results remain if only that facet changes.
		if mask|(1<<i) == allWorkspaceFacets {
			for _, value := range values[i] {
				counts[id][value]++
			}
		}
	}
}

func workspaceFacetOptions(selected map[string][]string, counts map[string]map[string]int) map[string][]string {
	options := make(map[string][]string, len(workspaceFacets))
	for _, id := range workspaceFacets {
		values := append([]string{}, selected[id]...)
		for value := range counts[id] {
			values = append(values, value)
		}
		slices.Sort(values)
		options[id] = slices.Compact(values)
	}
	return options
}
