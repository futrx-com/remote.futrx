package provisioning

import (
	"errors"
	"path"
	"strings"
	"time"
)

type InstallMode string

const (
	InstallWithNPM         InstallMode = "npm"
	InstallWithImageRepair InstallMode = "image-repair"
	// InstallWithScript runs the spec's InstallScript in the target execution
	// environment. It supports CLIs that ship as standalone binaries rather
	// than npm packages.
	InstallWithScript InstallMode = "script"
)

// CLISpec is the provider-owned description of a CLI installed on the host
// and/or in project containers. Each execution-environment integration owns
// how the shared policy is applied.
type CLISpec struct {
	Name       string
	ImageLabel string
	Binary     string
	// VersionArgs are passed to Binary when reporting or checking its
	// installed version. Output must contain a parseable semver; each execution
	// environment owns whether readiness requires the exact pin or a compatible
	// version at least as new as that pin.
	VersionArgs        []string
	PackageName        string
	Version            string
	ReportVersion      bool
	CheckVersion       bool
	VerifyAfterInstall bool
	InstallMode        InstallMode
	// InstallScript is the bash program used by InstallWithScript. It must be
	// self-contained, idempotent, and pinned to Version.
	InstallScript  string
	InstallTimeout time.Duration
	WaitTimeout    time.Duration
}

func (s CLISpec) NPMPackage() string {
	if s.Version == "" {
		return s.PackageName
	}
	return s.PackageName + "@" + s.Version
}

type CredentialFile struct {
	HostPath      string
	ContainerPath string
	Mode          string
	PushRequired  bool
	PullRequired  bool
}

// CredentialDirectory describes a dynamic directory of credential files.
// It supports providers that rotate an unknown set of files rather than one
// fixed credential document.
type CredentialDirectory struct {
	HostPath                 string
	ContainerPath            string
	ContainerDirs            []string
	AllowContainerOnly       bool
	MissingErrorFormat       string
	SyncOnlyWhenHostHasFiles bool
	SyncUnavailableIsNoop    bool
}

// CredentialSpec is provider policy for credential placement. The container
// integration performs the file transfer without knowing which provider owns
// the paths.
type CredentialSpec struct {
	Name          string
	HostDir       string
	ContainerDir  string
	Files         []CredentialFile
	LegacyDevices []string
	Directory     *CredentialDirectory
	SeedOnLaunch  bool
}

func (s CredentialSpec) Empty() bool {
	return len(s.Files) == 0 && s.Directory == nil
}

type InstructionTarget struct {
	Path     string
	HashPath string
}

type WorkspaceSkills struct {
	WorkspaceHome string
	HomeSkillsDir string
}

type TemplateFile struct {
	Content       []byte
	Path          string
	HashPath      string
	Mode          string
	Directory     string
	DirectoryMode string
}

// PersistentDirectory declares provider state that must survive project
// container replacement. HostDirectory is a single directory name below the
// project's private agent-home root; ContainerPath is the absolute location
// used by the provider CLI. Device must be a stable LXD disk-device name.
type PersistentDirectory struct {
	Device        string
	HostDirectory string
	ContainerPath string
}

func (d PersistentDirectory) Validate() error {
	if !validPersistentName(d.Device) || d.Device == "workspace" {
		return errors.New("invalid or reserved device name")
	}
	if !validPersistentName(d.HostDirectory) {
		return errors.New("invalid host directory name")
	}
	if !path.IsAbs(d.ContainerPath) || path.Clean(d.ContainerPath) != d.ContainerPath ||
		d.ContainerPath == "/root" || !strings.HasPrefix(d.ContainerPath, "/root/") {
		return errors.New("container path must be a clean path below /root")
	}
	return nil
}

func validPersistentName(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for index := 1; index < len(value); index++ {
		char := value[index]
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' {
			continue
		}
		return false
	}
	return true
}

// Profile is the provisioning policy supplied by one agent. CLI policy may be
// consumed for host and project execution; credential, mount, instruction,
// skill, and browser policy applies to project containers. No LXC, process,
// or filesystem operation lives here.
type Profile struct {
	ID                  string
	CLI                 CLISpec
	Credentials         CredentialSpec
	PersistentState     []PersistentDirectory
	Instructions        *InstructionTarget
	WorkspaceSkills     *WorkspaceSkills
	BrowserMCPTemplates []TemplateFile
}

func (p Profile) Clone() Profile {
	p.CLI.VersionArgs = append([]string(nil), p.CLI.VersionArgs...)
	p.Credentials.Files = append([]CredentialFile(nil), p.Credentials.Files...)
	p.Credentials.LegacyDevices = append([]string(nil), p.Credentials.LegacyDevices...)
	p.PersistentState = append([]PersistentDirectory(nil), p.PersistentState...)
	if p.Credentials.Directory != nil {
		directory := *p.Credentials.Directory
		directory.ContainerDirs = append([]string(nil), directory.ContainerDirs...)
		p.Credentials.Directory = &directory
	}
	if p.Instructions != nil {
		instructions := *p.Instructions
		p.Instructions = &instructions
	}
	if p.WorkspaceSkills != nil {
		skills := *p.WorkspaceSkills
		p.WorkspaceSkills = &skills
	}
	p.BrowserMCPTemplates = append([]TemplateFile(nil), p.BrowserMCPTemplates...)
	for i := range p.BrowserMCPTemplates {
		p.BrowserMCPTemplates[i].Content = append([]byte(nil), p.BrowserMCPTemplates[i].Content...)
	}
	return p
}
