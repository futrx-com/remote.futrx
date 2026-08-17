package templates

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
)

// definitionFile is the manifest every template directory must contain.
const definitionFile = "template.json"

// templateFiles holds every shipped template. Each immediate subdirectory is
// one template whose directory name must match its declared name.
//
//go:embed blank laravel node python wordpress
var templateFiles embed.FS

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// order pins the presentation order of the shipped templates; anything not
// listed sorts alphabetically after them. Keeping "blank" first makes the
// default the first card in the picker.
var order = map[string]int{
	"blank":     0,
	"wordpress": 1,
	"laravel":   2,
	"node":      3,
	"python":    4,
}

// Catalog is the validated, immutable set of shipped templates.
type Catalog struct {
	byName map[string]Template
	names  []string
}

// MustLoad loads the embedded catalog and panics on a malformed template.
// Template data is compiled into the binary, so a failure here is a build
// defect rather than an operational condition.
func MustLoad() *Catalog {
	catalog, err := Load()
	if err != nil {
		panic("load project templates: " + err.Error())
	}
	return catalog
}

// Load parses and validates every embedded template.
func Load() (*Catalog, error) {
	return LoadFS(templateFiles)
}

// LoadFS parses and validates templates from any filesystem laid out like the
// embedded one. Exported for tests that exercise validation with fixtures.
func LoadFS(files fs.FS) (*Catalog, error) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil, fmt.Errorf("read template root: %w", err)
	}
	catalog := &Catalog{byName: make(map[string]Template, len(entries))}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		template, err := loadTemplate(files, entry.Name())
		if err != nil {
			return nil, err
		}
		if _, duplicate := catalog.byName[template.Name]; duplicate {
			return nil, fmt.Errorf("template %q is declared twice", template.Name)
		}
		catalog.byName[template.Name] = template
		catalog.names = append(catalog.names, template.Name)
	}
	if _, ok := catalog.byName[DefaultName]; !ok {
		return nil, fmt.Errorf("default template %q is missing", DefaultName)
	}
	sort.Slice(catalog.names, func(i, j int) bool {
		left, leftKnown := order[catalog.names[i]]
		right, rightKnown := order[catalog.names[j]]
		if leftKnown != rightKnown {
			return leftKnown
		}
		if leftKnown && left != right {
			return left < right
		}
		return catalog.names[i] < catalog.names[j]
	})
	return catalog, nil
}

func loadTemplate(files fs.FS, dir string) (Template, error) {
	raw, err := fs.ReadFile(files, path.Join(dir, definitionFile))
	if err != nil {
		return Template{}, fmt.Errorf("template %q: read %s: %w", dir, definitionFile, err)
	}
	var definition Definition
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&definition); err != nil {
		return Template{}, fmt.Errorf("template %q: parse %s: %w", dir, definitionFile, err)
	}
	template := Template{Definition: definition}
	if err := validateDefinition(dir, definition); err != nil {
		return Template{}, err
	}
	if definition.ProvisionScript != "" {
		script, err := readTemplateFile(files, dir, definition.ProvisionScript)
		if err != nil {
			return Template{}, fmt.Errorf("template %q: provision script: %w", dir, err)
		}
		template.Script = script
	}
	if definition.AgentInstructions != "" {
		instructions, err := readTemplateFile(files, dir, definition.AgentInstructions)
		if err != nil {
			return Template{}, fmt.Errorf("template %q: agent instructions: %w", dir, err)
		}
		template.Instructions = instructions
	}
	for _, seed := range definition.SeedFiles {
		content, err := readTemplateFile(files, dir, seed.Source)
		if err != nil {
			return Template{}, fmt.Errorf("template %q: seed %s: %w", dir, seed.Source, err)
		}
		mode := seed.Mode
		if mode == "" {
			mode = "644"
		}
		template.Seeds = append(template.Seeds, Seed{
			Target:  seed.Target,
			Mode:    mode,
			Content: content,
		})
	}
	return template, nil
}

func validateDefinition(dir string, definition Definition) error {
	if !namePattern.MatchString(definition.Name) {
		return fmt.Errorf("template %q: name %q must match %s", dir, definition.Name, namePattern)
	}
	if definition.Name != dir {
		return fmt.Errorf("template %q: name %q does not match its directory", dir, definition.Name)
	}
	if strings.TrimSpace(definition.Title) == "" {
		return fmt.Errorf("template %q: title is required", dir)
	}
	if strings.TrimSpace(definition.Description) == "" {
		return fmt.Errorf("template %q: description is required", dir)
	}
	if strings.TrimSpace(definition.Icon) == "" {
		return fmt.Errorf("template %q: icon is required", dir)
	}
	for _, port := range definition.DefaultPorts {
		if port < 1 || port > 65535 {
			return fmt.Errorf("template %q: default port %d is out of range", dir, port)
		}
	}
	targets := make(map[string]bool, len(definition.SeedFiles))
	for _, seed := range definition.SeedFiles {
		if err := validateSeedTarget(seed.Target); err != nil {
			return fmt.Errorf("template %q: seed %q: %w", dir, seed.Source, err)
		}
		if targets[seed.Target] {
			return fmt.Errorf("template %q: seed target %q is declared twice", dir, seed.Target)
		}
		targets[seed.Target] = true
	}
	return nil
}

// validateSeedTarget keeps seeds inside the durable workspace mount. Writing
// outside it would be lost on the next container replacement, and an escaping
// path would let a template write anywhere in the rootfs.
func validateSeedTarget(target string) error {
	if target == "" {
		return fmt.Errorf("target is required")
	}
	if target != path.Clean(target) || !strings.HasPrefix(target, "/workspace/") {
		return fmt.Errorf("target %q must be a clean absolute path under /workspace", target)
	}
	for _, element := range strings.Split(target, "/") {
		if element == ".." {
			return fmt.Errorf("target %q must not traverse upwards", target)
		}
	}
	return nil
}

func readTemplateFile(files fs.FS, dir, name string) ([]byte, error) {
	if name != path.Clean(name) || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "..") {
		return nil, fmt.Errorf("%q must be a relative file name inside the template directory", name)
	}
	content, err := fs.ReadFile(files, path.Join(dir, name))
	if err != nil {
		return nil, err
	}
	if len(content) == 0 {
		return nil, fmt.Errorf("%q is empty", name)
	}
	return content, nil
}

// Names returns every template name in presentation order.
func (c *Catalog) Names() []string {
	names := make([]string, len(c.names))
	copy(names, c.names)
	return names
}

// List returns every template in presentation order.
func (c *Catalog) List() []Template {
	out := make([]Template, 0, len(c.names))
	for _, name := range c.names {
		out = append(out, c.byName[name])
	}
	return out
}

// Get returns a template by name.
func (c *Catalog) Get(name string) (Template, bool) {
	template, ok := c.byName[NormalizeName(name)]
	return template, ok
}

// Resolve returns the template for name, falling back to the default for an
// empty name (projects created before templates existed) and for a name that
// is no longer shipped. Provisioning must never fail because metadata refers
// to a template that has since been removed.
func (c *Catalog) Resolve(name string) Template {
	if template, ok := c.Get(name); ok {
		return template
	}
	return c.byName[DefaultName]
}

// Has reports whether name is a known template.
func (c *Catalog) Has(name string) bool {
	_, ok := c.Get(name)
	return ok
}

// DefaultName returns the template assigned when a caller requests none.
func (c *Catalog) DefaultName() string {
	return DefaultName
}
