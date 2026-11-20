package provisioning

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

const defaultSkillsAssetRoot = "assets/skills"

//go:embed assets/skills
var defaultSkillsFS embed.FS

// DefaultSkillAsset is one file within a code-owned default skill.
type DefaultSkillAsset struct {
	Path    string
	Content []byte
}

// DefaultSkill is a code-owned skill published into every project workspace.
// Command is both its directory name and its stable picker/invocation name.
type DefaultSkill struct {
	Command string
	Assets  []DefaultSkillAsset
}

var embeddedDefaultSkills = loadDefaultSkills()

// DefaultSkills returns an isolated copy of every embedded default skill.
func DefaultSkills() []DefaultSkill {
	skills := make([]DefaultSkill, len(embeddedDefaultSkills))
	for i, skill := range embeddedDefaultSkills {
		skills[i].Command = skill.Command
		skills[i].Assets = make([]DefaultSkillAsset, len(skill.Assets))
		for j, asset := range skill.Assets {
			skills[i].Assets[j] = DefaultSkillAsset{
				Path:    asset.Path,
				Content: append([]byte(nil), asset.Content...),
			}
		}
	}
	return skills
}

func loadDefaultSkills() []DefaultSkill {
	entries, err := fs.ReadDir(defaultSkillsFS, defaultSkillsAssetRoot)
	if err != nil {
		panic(fmt.Sprintf("read embedded default skills: %v", err))
	}

	skills := make([]DefaultSkill, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		command := entry.Name()
		root := path.Join(defaultSkillsAssetRoot, command)
		skill := DefaultSkill{Command: command}
		err := fs.WalkDir(defaultSkillsFS, root, func(assetPath string, asset fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if asset.IsDir() {
				if assetPath != root && strings.HasPrefix(asset.Name(), ".") {
					return fs.SkipDir
				}
				return nil
			}
			relativePath := strings.TrimPrefix(assetPath, root+"/")
			if relativePath == assetPath || relativePath == "" || path.IsAbs(relativePath) || strings.HasPrefix(path.Clean(relativePath), "../") {
				return fmt.Errorf("unsafe embedded skill path %q", assetPath)
			}
			content, err := defaultSkillsFS.ReadFile(assetPath)
			if err != nil {
				return err
			}
			skill.Assets = append(skill.Assets, DefaultSkillAsset{Path: relativePath, Content: content})
			return nil
		})
		if err != nil {
			panic(fmt.Sprintf("read embedded default skill %q: %v", command, err))
		}
		if !hasSkillEntry(skill.Assets) {
			panic(fmt.Sprintf("embedded default skill %q has no SKILL.md", command))
		}
		sort.Slice(skill.Assets, func(i, j int) bool {
			return skill.Assets[i].Path < skill.Assets[j].Path
		})
		skills = append(skills, skill)
	}
	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Command < skills[j].Command
	})
	return skills
}

func hasSkillEntry(assets []DefaultSkillAsset) bool {
	for _, asset := range assets {
		if asset.Path == "SKILL.md" {
			return true
		}
	}
	return false
}
