package codex

import (
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
	harnessauth "github.com/futrx-com/remote.futrx.com/internal/service/harness/auth"
)

func TestNativeForkAndResumeRetainSourceAccountHome(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	saved := harnessauth.RunCredential{AccountID: "account", Credential: codexTestCredential("account", "test")}
	source, err := newAccountRun(harness.RunRequest{ConversationID: "source-chat", ProjectID: "project"}, saved)
	if err != nil {
		t.Fatal(err)
	}
	for _, fork := range []bool{true, false} {
		run, err := newAccountRun(harness.RunRequest{
			ConversationID: "fork-chat", NativeConversationID: "source-chat", ProjectID: "project", Fork: fork,
		}, saved)
		if err != nil {
			t.Fatal(err)
		}
		if run.hostHome != source.hostHome || run.containerHome != source.containerHome {
			t.Fatalf("native %t home = %q / %q, source home = %q / %q", fork, run.hostHome, run.containerHome, source.hostHome, source.containerHome)
		}
	}
	other, err := newAccountRun(harness.RunRequest{
		ConversationID: "fork-chat", NativeConversationID: "source-chat", ProjectID: "project",
	}, harnessauth.RunCredential{AccountID: "other", Credential: codexTestCredential("other", "test")})
	if err != nil {
		t.Fatal(err)
	}
	if other.hostHome == source.hostHome || other.containerHome == source.containerHome {
		t.Fatal("native source scope crossed account isolation")
	}
}
