package antigravity

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Print mode never reports which conversation it created (upstream issue #7),
// but every conversation materializes as ~/.gemini/antigravity-cli/brain/<id>/.
// Snapshotting that directory around a run recovers the id so later prompts
// can resume with --conversation instead of starting fresh.

const sessionProbeTimeout = 10 * time.Second

var conversationIDPattern = regexp.MustCompile(`^[0-9a-fA-F-]{8,64}$`)

type conversationStore struct {
	containerName string
}

func (s conversationStore) brainDir() string {
	if s.containerName != "" {
		return containerAgentHome + "/" + stateDirUnderHome + "/brain"
	}
	home := os.Getenv("HOME")
	if home == "" {
		home = "/root"
	}
	return filepath.Join(home, stateDirUnderHome, "brain")
}

// list returns the conversation ids present in the brain directory. A missing
// directory (not signed in yet, or nothing run) is an empty listing, not an
// error.
func (s conversationStore) list(ctx context.Context) map[string]struct{} {
	ids := map[string]struct{}{}
	for _, name := range s.entries(ctx) {
		name = strings.TrimSpace(name)
		if conversationIDPattern.MatchString(name) {
			ids[name] = struct{}{}
		}
	}
	return ids
}

// newConversation reports the id that exists now but was absent from the
// pre-run snapshot. Ambiguity (zero or several new ids) returns "".
func (s conversationStore) newConversation(ctx context.Context, before map[string]struct{}) string {
	fresh := make([]string, 0, 1)
	for id := range s.list(ctx) {
		if _, existed := before[id]; !existed {
			fresh = append(fresh, id)
		}
	}
	if len(fresh) != 1 {
		if len(fresh) > 1 {
			// Concurrent runs on the same home are possible in principle; an
			// arbitrary pick could cross-wire chats, so decline to resume.
			sort.Strings(fresh)
		}
		return ""
	}
	return fresh[0]
}

func (s conversationStore) entries(ctx context.Context) []string {
	probeCtx, cancel := context.WithTimeout(ctx, sessionProbeTimeout)
	defer cancel()

	if s.containerName == "" {
		items, err := os.ReadDir(s.brainDir())
		if err != nil {
			return nil
		}
		names := make([]string, 0, len(items))
		for _, item := range items {
			if item.IsDir() {
				names = append(names, item.Name())
			}
		}
		return names
	}

	out, err := exec.CommandContext(
		probeCtx,
		"lxc", "exec", s.containerName, "--",
		"sh", "-c", "ls -1 "+s.brainDir()+" 2>/dev/null",
	).Output()
	if err != nil {
		return nil
	}
	return strings.Split(string(out), "\n")
}
