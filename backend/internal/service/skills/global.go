package skills

// The global skills library is the platform-wide counterpart of a project's
// own .agents/skills tree: admins author skill directories once and every
// project sees them in its picker. Storage stays in the same SKILL.md
// directory format so a global skill can be copied to or from a project
// workspace without translation.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

const (
	// SourceGlobal marks catalog entries served from the global library. It
	// is part of a selected skill's identity, so it must stay stable.
	SourceGlobal = "global"
	// ScopeGlobal is the scope flag the frontend badges in the picker.
	ScopeGlobal = "global"

	// MaxGlobalSkillFiles bounds one skill's file count.
	MaxGlobalSkillFiles = 64
	// MaxGlobalSkillFileBytes bounds a single file inside a skill.
	MaxGlobalSkillFileBytes = 256 * 1024
	// MaxGlobalSkillBytes bounds one skill's total size.
	MaxGlobalSkillBytes = 1024 * 1024
	// MaxGlobalSkillNameLength bounds the directory name of a skill.
	MaxGlobalSkillNameLength = 64
)

var (
	ErrGlobalLibraryUnavailable = errors.New("global skills library unavailable")
	ErrGlobalSkillNotFound      = errors.New("global skill not found")
	ErrGlobalSkillExists        = errors.New("global skill already exists")
	ErrInvalidGlobalSkillName   = errors.New(
		"skill name must be 1-64 characters of lowercase letters, digits, '.', '_' or '-'",
	)
	ErrInvalidGlobalSkillFile = errors.New(
		"skill file paths must be relative, slash separated, and must not escape the skill directory",
	)
	ErrMissingSkillManifest = errors.New("a skill must contain a SKILL.md file")
	ErrGlobalSkillTooLarge  = errors.New("skill exceeds the global library size limits")
	ErrProjectSkillNotFound = errors.New("project skill not found")
)

// reservedGlobalSkillNames cannot be used as skill directory names: the admin
// API reserves them for actions on the collection.
var reservedGlobalSkillNames = map[string]bool{"import": true}

// GlobalRecord is the persistence shape of one library entry. Files is nil on
// listing reads, where contents are deliberately not loaded.
type GlobalRecord struct {
	Name      string
	Files     map[string][]byte
	FileNames []string
	AlwaysOn  bool
	UpdatedAt int64
}

// GlobalRepository is the file-backed library port implemented by
// stores/fileskillsglobal.
type GlobalRepository interface {
	List(ctx context.Context) ([]GlobalRecord, error)
	Get(ctx context.Context, name string) (GlobalRecord, error)
	Save(ctx context.Context, record GlobalRecord) (GlobalRecord, error)
	SetAlwaysOn(ctx context.Context, name string, alwaysOn bool) (GlobalRecord, error)
	Delete(ctx context.Context, name string) error
}

// GlobalSkill is the transport shape of a library entry. Files is populated
// only on single-entry reads.
type GlobalSkill struct {
	Name        string            `json:"name"`
	Title       string            `json:"title,omitempty"`
	Description string            `json:"description,omitempty"`
	AlwaysOn    bool              `json:"alwaysOn"`
	UpdatedAt   int64             `json:"updatedAt,omitempty"`
	FileNames   []string          `json:"fileNames,omitempty"`
	Files       map[string]string `json:"files,omitempty"`
}

// GlobalInput creates or replaces one library entry.
type GlobalInput struct {
	Name     string
	Files    map[string]string
	AlwaysOn bool
}

// GlobalUpdate patches one library entry. Nil fields are left untouched, so
// the "always on" toggle does not have to resend the file set.
type GlobalUpdate struct {
	Files    map[string]string
	AlwaysOn *bool
}

// GlobalService owns global-library policy: validation, seeding, the merge
// into a project's catalog, and copying a project skill into the library.
type GlobalService struct {
	repo     GlobalRepository
	projects ProjectCatalog
}

// NewGlobalService returns the library service. projects may be nil, which
// only disables the "import from project" action.
func NewGlobalService(repo GlobalRepository, projects ProjectCatalog) *GlobalService {
	if repo == nil {
		return nil
	}
	return &GlobalService{repo: repo, projects: projects}
}

// List returns every library entry with its parsed SKILL.md metadata.
func (s *GlobalService) List(ctx context.Context) ([]GlobalSkill, error) {
	records, err := s.records(ctx)
	if err != nil {
		return nil, err
	}
	skills := make([]GlobalSkill, 0, len(records))
	for _, record := range records {
		skills = append(skills, s.describe(record))
	}
	return skills, nil
}

// Get returns one library entry including every file it owns.
func (s *GlobalService) Get(ctx context.Context, name string) (GlobalSkill, error) {
	if s == nil || s.repo == nil {
		return GlobalSkill{}, ErrGlobalLibraryUnavailable
	}
	record, err := s.repo.Get(ctx, NormalizeGlobalSkillName(name))
	if err != nil {
		return GlobalSkill{}, err
	}
	skill := s.describe(record)
	skill.Files = map[string]string{}
	for relative, content := range record.Files {
		skill.Files[relative] = string(content)
	}
	return skill, nil
}

// Create adds a new library entry and rejects a name that already exists.
func (s *GlobalService) Create(ctx context.Context, in GlobalInput) (GlobalSkill, error) {
	if s == nil || s.repo == nil {
		return GlobalSkill{}, ErrGlobalLibraryUnavailable
	}
	name := NormalizeGlobalSkillName(in.Name)
	if !ValidGlobalSkillName(name) {
		return GlobalSkill{}, ErrInvalidGlobalSkillName
	}
	if _, err := s.repo.Get(ctx, name); err == nil {
		return GlobalSkill{}, ErrGlobalSkillExists
	} else if !errors.Is(err, ErrGlobalSkillNotFound) {
		return GlobalSkill{}, err
	}
	files, err := NormalizeGlobalSkillFiles(in.Files)
	if err != nil {
		return GlobalSkill{}, err
	}
	return s.save(ctx, GlobalRecord{Name: name, Files: files, AlwaysOn: in.AlwaysOn})
}

// Update replaces the files and/or the "always on" flag of an existing entry.
func (s *GlobalService) Update(ctx context.Context, name string, in GlobalUpdate) (GlobalSkill, error) {
	if s == nil || s.repo == nil {
		return GlobalSkill{}, ErrGlobalLibraryUnavailable
	}
	name = NormalizeGlobalSkillName(name)
	current, err := s.repo.Get(ctx, name)
	if err != nil {
		return GlobalSkill{}, err
	}

	if in.Files == nil {
		if in.AlwaysOn == nil {
			return s.describe(current), nil
		}
		record, err := s.repo.SetAlwaysOn(ctx, name, *in.AlwaysOn)
		if err != nil {
			return GlobalSkill{}, err
		}
		return s.describe(record), nil
	}

	files, err := NormalizeGlobalSkillFiles(in.Files)
	if err != nil {
		return GlobalSkill{}, err
	}
	alwaysOn := current.AlwaysOn
	if in.AlwaysOn != nil {
		alwaysOn = *in.AlwaysOn
	}
	return s.save(ctx, GlobalRecord{Name: name, Files: files, AlwaysOn: alwaysOn})
}

// Delete removes an entry from the library. Containers drop the matching link
// on their next skill sync.
func (s *GlobalService) Delete(ctx context.Context, name string) error {
	if s == nil || s.repo == nil {
		return ErrGlobalLibraryUnavailable
	}
	return s.repo.Delete(ctx, NormalizeGlobalSkillName(name))
}

// ImportFromProject copies a project's own skill directory into the library.
// The caller is admin-gated by the handler, matching the catalog's rule that
// an admin may read any project.
func (s *GlobalService) ImportFromProject(
	ctx context.Context,
	projectID serviceproject.ID,
	skillName string,
	targetName string,
	alwaysOn bool,
) (GlobalSkill, error) {
	if s == nil || s.repo == nil {
		return GlobalSkill{}, ErrGlobalLibraryUnavailable
	}
	if s.projects == nil {
		return GlobalSkill{}, ErrProjectLookupUnavailable
	}
	project, err := s.projects.Get(ctx, projectID)
	if err != nil {
		if errors.Is(err, serviceproject.ErrNotFound) {
			return GlobalSkill{}, ErrProjectNotFound
		}
		return GlobalSkill{}, err
	}

	source := NormalizeGlobalSkillName(skillName)
	if !ValidGlobalSkillName(source) {
		return GlobalSkill{}, ErrInvalidGlobalSkillName
	}
	files, err := readProjectSkill(project.Cwd, source)
	if err != nil {
		return GlobalSkill{}, err
	}

	name := NormalizeGlobalSkillName(targetName)
	if name == "" {
		name = source
	}
	if !ValidGlobalSkillName(name) {
		return GlobalSkill{}, ErrInvalidGlobalSkillName
	}
	if _, err := s.repo.Get(ctx, name); err == nil {
		return GlobalSkill{}, ErrGlobalSkillExists
	} else if !errors.Is(err, ErrGlobalSkillNotFound) {
		return GlobalSkill{}, err
	}
	normalized, err := NormalizeGlobalSkillFiles(files)
	if err != nil {
		return GlobalSkill{}, err
	}
	return s.save(ctx, GlobalRecord{Name: name, Files: normalized, AlwaysOn: alwaysOn})
}

// CatalogEntries renders the library as picker entries for one provider.
// Entries whose command collides with a container-resident skill are marked
// shadowed: the project-local directory wins inside the container, so the
// global copy is displayed but not selectable.
func (s *GlobalService) CatalogEntries(
	ctx context.Context,
	provider Provider,
	existing []Skill,
) []Skill {
	records, err := s.records(ctx)
	if err != nil || len(records) == 0 {
		return nil
	}
	occupied := map[string]bool{}
	for _, skill := range existing {
		if !shadowsGlobalSkill(skill) {
			continue
		}
		occupied[strings.ToLower(strings.TrimSpace(skill.Command))] = true
	}

	entries := make([]Skill, 0, len(records))
	for _, record := range records {
		metadata := parseSkillMetadata(record.Files[SkillFileName])
		name := strings.TrimSpace(metadata.Name)
		if name == "" {
			name = record.Name
		}
		entries = append(entries, Skill{
			Name:        name,
			Command:     record.Name,
			Description: metadata.Description,
			Provider:    provider,
			Source:      SourceGlobal,
			Scope:       ScopeGlobal,
			ReadOnly:    true,
			AlwaysOn:    record.AlwaysOn,
			Shadowed:    occupied[strings.ToLower(record.Name)],
		})
	}
	return entries
}

// DefaultSkills returns the library entries an admin marked "always on".
// New project chats start with them selected.
func (s *GlobalService) DefaultSkills(ctx context.Context, provider Provider) ([]Skill, error) {
	records, err := s.records(ctx)
	if err != nil {
		return nil, err
	}
	defaults := make([]Skill, 0, len(records))
	for _, record := range records {
		if !record.AlwaysOn {
			continue
		}
		metadata := parseSkillMetadata(record.Files[SkillFileName])
		name := strings.TrimSpace(metadata.Name)
		if name == "" {
			name = record.Name
		}
		defaults = append(defaults, Skill{
			Name:     name,
			Command:  record.Name,
			Provider: provider,
			Source:   SourceGlobal,
			Scope:    ScopeGlobal,
			ReadOnly: true,
			AlwaysOn: true,
		})
	}
	return defaults, nil
}

func (s *GlobalService) save(ctx context.Context, record GlobalRecord) (GlobalSkill, error) {
	record.UpdatedAt = time.Now().UnixMilli()
	saved, err := s.repo.Save(ctx, record)
	if err != nil {
		return GlobalSkill{}, err
	}
	skill := s.describe(saved)
	skill.Files = map[string]string{}
	for relative, content := range saved.Files {
		skill.Files[relative] = string(content)
	}
	return skill, nil
}

// records lists the library with SKILL.md contents loaded, which the metadata
// parse needs. The listing read omits contents, so SKILL.md is fetched per
// entry; libraries are small and admin-managed.
func (s *GlobalService) records(ctx context.Context) ([]GlobalRecord, error) {
	if s == nil || s.repo == nil {
		return nil, ErrGlobalLibraryUnavailable
	}
	listed, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	records := make([]GlobalRecord, 0, len(listed))
	for _, record := range listed {
		if record.Files == nil {
			full, err := s.repo.Get(ctx, record.Name)
			if err != nil {
				if errors.Is(err, ErrGlobalSkillNotFound) {
					continue
				}
				return nil, err
			}
			record.Files = full.Files
		}
		records = append(records, record)
	}
	return records, nil
}

func (s *GlobalService) describe(record GlobalRecord) GlobalSkill {
	metadata := parseSkillMetadata(record.Files[SkillFileName])
	return GlobalSkill{
		Name:        record.Name,
		Title:       strings.TrimSpace(metadata.Name),
		Description: metadata.Description,
		AlwaysOn:    record.AlwaysOn,
		UpdatedAt:   record.UpdatedAt,
		FileNames:   record.FileNames,
	}
}

// shadowsGlobalSkill reports whether a catalog entry occupies the container's
// /workspace/.agents/skills/<command> slot. Host-level user skills live
// outside the project container and therefore never shadow a global skill.
func shadowsGlobalSkill(skill Skill) bool {
	switch strings.ToLower(strings.TrimSpace(skill.Source)) {
	case "project", "remote":
		return true
	default:
		return false
	}
}

// NormalizeGlobalSkillName lowercases and trims a candidate directory name.
func NormalizeGlobalSkillName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// ValidGlobalSkillName reports whether a name is safe as a directory name on
// the host, inside a container, and as a provider skill trigger.
func ValidGlobalSkillName(name string) bool {
	if name == "" || len(name) > MaxGlobalSkillNameLength {
		return false
	}
	if name == "." || name == ".." {
		return false
	}
	for index := 0; index < len(name); index++ {
		char := name[index]
		switch {
		case char >= 'a' && char <= 'z':
		case char >= '0' && char <= '9':
		case char == '-' || char == '_' || char == '.':
		default:
			return false
		}
	}
	// A leading dot would hide the directory from every skill walker, and a
	// leading underscore collides with the library's reserved children.
	return name[0] != '.' && name[0] != '_' && !reservedGlobalSkillNames[name]
}

// NormalizeGlobalSkillFiles validates an uploaded file set and converts it to
// the byte map the store persists. Paths are cleaned to forward-slash relative
// form; anything that escapes the skill directory is rejected.
func NormalizeGlobalSkillFiles(files map[string]string) (map[string][]byte, error) {
	if len(files) == 0 {
		return nil, ErrMissingSkillManifest
	}
	if len(files) > MaxGlobalSkillFiles {
		return nil, ErrGlobalSkillTooLarge
	}

	normalized := make(map[string][]byte, len(files))
	total := 0
	for rawPath, content := range files {
		relative, err := NormalizeGlobalSkillFilePath(rawPath)
		if err != nil {
			return nil, err
		}
		if len(content) > MaxGlobalSkillFileBytes {
			return nil, ErrGlobalSkillTooLarge
		}
		total += len(content)
		if total > MaxGlobalSkillBytes {
			return nil, ErrGlobalSkillTooLarge
		}
		if _, duplicate := normalized[relative]; duplicate {
			return nil, ErrInvalidGlobalSkillFile
		}
		normalized[relative] = []byte(content)
	}
	if _, ok := normalized[SkillFileName]; !ok {
		return nil, ErrMissingSkillManifest
	}
	return normalized, nil
}

// NormalizeGlobalSkillFilePath cleans one relative path inside a skill.
func NormalizeGlobalSkillFilePath(rawPath string) (string, error) {
	candidate := strings.TrimSpace(strings.ReplaceAll(rawPath, "\\", "/"))
	candidate = strings.TrimPrefix(candidate, "./")
	if candidate == "" || strings.HasPrefix(candidate, "/") {
		return "", ErrInvalidGlobalSkillFile
	}
	cleaned := path.Clean(candidate)
	if cleaned == "." || cleaned != candidate {
		return "", ErrInvalidGlobalSkillFile
	}
	for _, segment := range strings.Split(cleaned, "/") {
		if segment == "" || segment == "." || segment == ".." || strings.HasPrefix(segment, ".") {
			return "", ErrInvalidGlobalSkillFile
		}
	}
	return cleaned, nil
}

// readProjectSkill loads a project's skill directory from the host workspace,
// searching the canonical .agents root before the legacy provider fallbacks.
func readProjectSkill(workspace, name string) (map[string]string, error) {
	if strings.TrimSpace(workspace) == "" {
		return nil, ErrProjectSkillNotFound
	}
	for _, root := range projectRoots(workspace) {
		directory := filepath.Join(root.path, name)
		if _, err := os.Stat(filepath.Join(directory, SkillFileName)); err != nil {
			continue
		}
		files, err := readSkillDirectory(directory)
		if err != nil {
			return nil, err
		}
		return files, nil
	}
	return nil, ErrProjectSkillNotFound
}

// readSkillDirectory reads every regular, non-hidden file of a skill folder
// into a relative-path map, enforcing the same limits as an upload.
func readSkillDirectory(root string) (map[string]string, error) {
	files := map[string]string{}
	total := 0
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if current != root && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() || strings.HasPrefix(entry.Name(), ".") {
			return nil
		}
		relative, relErr := filepath.Rel(root, current)
		if relErr != nil {
			return relErr
		}
		if len(files) >= MaxGlobalSkillFiles {
			return ErrGlobalSkillTooLarge
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		if info.Size() > MaxGlobalSkillFileBytes {
			return ErrGlobalSkillTooLarge
		}
		total += int(info.Size())
		if total > MaxGlobalSkillBytes {
			return ErrGlobalSkillTooLarge
		}
		data, readErr := os.ReadFile(current)
		if readErr != nil {
			return readErr
		}
		files[filepath.ToSlash(relative)] = string(data)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read skill directory: %w", err)
	}
	if _, ok := files[SkillFileName]; !ok {
		return nil, ErrProjectSkillNotFound
	}
	return files, nil
}

// SortSkills orders a merged catalog the way the picker renders it.
func SortSkills(skills []Skill) {
	sort.SliceStable(skills, func(i, j int) bool {
		left := strings.ToLower(skills[i].Name)
		right := strings.ToLower(skills[j].Name)
		if left == right {
			return skills[i].Source < skills[j].Source
		}
		return left < right
	})
}
