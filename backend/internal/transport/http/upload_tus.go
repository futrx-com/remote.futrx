package httptransport

// Resumable file uploads via the tus protocol (https://tus.io).
//
// Flow:
//   1. Client POSTs to /api/uploads with Upload-Length + Upload-Metadata
//      (chatId + filename, base64 per the tus spec) — server returns a
//      Location pointing to the new upload's URL.
//   2. Client PATCHes that URL with the next chunk and an Upload-Offset
//      header; the server appends and returns the new offset. Repeated until
//      the byte total reaches Upload-Length.
//   3. Server emits a CompleteUploads event; our consumer moves the
//      finished blob from the chunk temp dir into the chat's workspace cwd
//      with the proper ownership + mode.
//
// Chunks live under <tmpRoot>/chunks (default /var/lib/remote/uploads/tmp);
// finished files are moved to the chat-resolved cwd. We intentionally do
// NOT use /tmp — it's tmpfs on this host and would RAM-back multi-GB uploads.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/tus/tusd/v2/pkg/filestore"
	tusd "github.com/tus/tusd/v2/pkg/handler"

	serviceaudit "github.com/futrx-com/remote.futrx.com/internal/service/audit"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

const (
	// containerRootHostUID mirrors the LXD unprivileged idmap so files
	// land in the workspace owned by container-root.
	containerRootHostUID = 1000000

	// 10 GiB max — adjust via UPLOAD_MAX_BYTES if needed.
	defaultUploadMaxBytes int64 = 10 << 30
)

// ChatUploadResolver resolves a chat id to the host cwd that uploads should
// land in. Satisfied by *servicechat.Service.
type ChatUploadResolver interface {
	UploadTarget(ctx context.Context, id servicechat.ID) (string, error)
}

// UploadGate decides whether a request can initiate an upload tied to a
// given chat. The handler resolves the chat to its project and asks. If nil
// is passed, no gating is performed (single-user dev mode).
type UploadGate interface {
	// CanUploadToChat returns true if the bearer of r is allowed to upload
	// to the chat with id `chatID`. Implementations should treat missing
	// session as "no" — return false, nil.
	CanUploadToChat(ctx context.Context, r *http.Request, chatID servicechat.ID) (bool, error)
}

// UploadHandler exposes the tus protocol at /api/uploads and consumes
// completion events asynchronously.
type UploadHandler struct {
	tus     *tusd.UnroutedHandler
	chats   ChatUploadResolver
	tmpRoot string
	gate    UploadGate
	audit   serviceaudit.Recorder
}

// WithAudit records the creation of an upload. Completion happens
// asynchronously on a tusd hook with no request context, so the audited moment
// is the POST that reserves the upload and names the caller.
func (u *UploadHandler) WithAudit(recorder serviceaudit.Recorder) *UploadHandler {
	u.audit = recorder
	return u
}

func NewUploadHandler(chats ChatUploadResolver, dataDir string) (*UploadHandler, error) {
	return NewUploadHandlerWithGate(chats, dataDir, nil)
}

// NewUploadHandlerWithGate is the auth-aware constructor; pass nil to
// preserve the old, ungated behavior.
func NewUploadHandlerWithGate(chats ChatUploadResolver, dataDir string, gate UploadGate) (*UploadHandler, error) {
	tmpRoot := filepath.Join(dataDir, "uploads", "tmp")
	if err := os.MkdirAll(tmpRoot, 0o700); err != nil {
		return nil, fmt.Errorf("create upload tmp: %w", err)
	}

	store := filestore.New(tmpRoot)
	composer := tusd.NewStoreComposer()
	store.UseIn(composer)

	maxBytes := defaultUploadMaxBytes
	if v := os.Getenv("UPLOAD_MAX_BYTES"); v != "" {
		var parsed int64
		if _, err := fmt.Sscan(v, &parsed); err == nil && parsed > 0 {
			maxBytes = parsed
		}
	}

	cfg := tusd.Config{
		BasePath:                "/api/uploads/",
		StoreComposer:           composer,
		MaxSize:                 maxBytes,
		NotifyCompleteUploads:   true,
		NotifyTerminatedUploads: true,
		RespectForwardedHeaders: true,
	}
	h, err := tusd.NewUnroutedHandler(cfg)
	if err != nil {
		return nil, fmt.Errorf("tusd handler: %w", err)
	}

	uh := &UploadHandler{tus: h, chats: chats, tmpRoot: tmpRoot, gate: gate}
	go uh.drainCompletions(h.CompleteUploads)
	go uh.drainTerminations(h.TerminatedUploads)
	return uh, nil
}

func (u *UploadHandler) RegisterRoutes(mux *http.ServeMux) {
	// tusd v2's extractIDFromPath does NOT strip the BasePath — it just
	// trims slashes off whatever r.URL.Path it sees. Without StripPrefix
	// the upload ID ends up as "api/uploads/<id>" and every PATCH 404s.
	handler := http.StripPrefix("/api/uploads", u.tus.Middleware(http.HandlerFunc(u.dispatch)))
	mux.Handle("/api/uploads", handler)
	mux.Handle("/api/uploads/", handler)
}

// dispatch routes one tus request to the right method handler. The tus
// middleware already filtered OPTIONS / version checks / CORS before we
// got here.
//
// Auth gate: on POST (= creating a new upload) we parse the Upload-Metadata
// header to find the target chatId, then ask the gate whether the caller
// can upload to it. PATCH/HEAD/GET/DELETE are NOT re-gated — they require
// the random upload ID which is only handed to the original POSTer. Project
// membership changes won't retroactively kill in-flight uploads, but that's
// acceptable for the chunk window (minutes, not hours).
func (u *UploadHandler) dispatch(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost && u.gate != nil {
		chatID := chatIDFromUploadMetadata(r.Header.Get("Upload-Metadata"))
		if chatID == "" {
			SendErr(w, http.StatusBadRequest, "upload missing chatId in Upload-Metadata")
			return
		}
		ok, err := u.gate.CanUploadToChat(r.Context(), r, servicechat.ID(chatID))
		if err != nil {
			SendErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !ok {
			SendErr(w, http.StatusForbidden, "not a member of this chat's project")
			return
		}
		u.recordUpload(r, chatID, nil)
	}

	switch r.Method {
	case http.MethodPost:
		u.tus.PostFile(w, r)
	case http.MethodHead:
		u.tus.HeadFile(w, r)
	case http.MethodPatch:
		u.tus.PatchFile(w, r)
	case http.MethodGet:
		u.tus.GetFile(w, r)
	case http.MethodDelete:
		u.tus.DelFile(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (u *UploadHandler) recordUpload(r *http.Request, chatID string, err error) {
	if u.audit == nil {
		return
	}
	u.audit.Record(r.Context(), serviceaudit.Result(
		serviceaudit.ActionWorkspaceFileUpload,
		serviceaudit.Target{Type: serviceaudit.TargetChat, ID: chatID},
		serviceaudit.Meta{"bytes": r.Header.Get("Upload-Length")},
		err,
	))
}

// chatIDFromUploadMetadata pulls the `chatId` field out of a tus
// Upload-Metadata header (per https://tus.io/protocols/resumable-upload —
// comma-separated `key b64value` pairs). Returns "" if missing/malformed.
func chatIDFromUploadMetadata(header string) string {
	for _, pair := range strings.Split(header, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		var k, v string
		if i := strings.IndexByte(pair, ' '); i >= 0 {
			k = strings.TrimSpace(pair[:i])
			v = strings.TrimSpace(pair[i+1:])
		} else {
			k = pair
		}
		if k != "chatId" {
			continue
		}
		if v == "" {
			return ""
		}
		raw, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(raw))
	}
	return ""
}

func (u *UploadHandler) drainCompletions(events <-chan tusd.HookEvent) {
	for ev := range events {
		if err := u.finalize(ev); err != nil {
			log.Printf("upload[%s] finalize: %v", ev.Upload.ID, err)
		}
	}
}

func (u *UploadHandler) drainTerminations(events <-chan tusd.HookEvent) {
	for ev := range events {
		// tusd's filestore already removes the .info + .bin when an upload
		// is DELETEd. We just log for visibility.
		log.Printf("upload[%s] terminated", ev.Upload.ID)
	}
}

// finalize moves the chunked file out of the tus tmpRoot and into the chat's
// .uploads dir under the workspace root. Chowns to the container-mapped uid.
func (u *UploadHandler) finalize(ev tusd.HookEvent) error {
	meta := ev.Upload.MetaData
	chatID := strings.TrimSpace(meta["chatId"])
	if chatID == "" {
		return errors.New("upload metadata missing chatId")
	}
	filename := strings.TrimSpace(meta["filename"])
	if filename == "" {
		return errors.New("upload metadata missing filename")
	}
	base := filepath.Base(filename)
	if base == "" || base == "." || base == ".." || strings.ContainsAny(base, "/\\") {
		return fmt.Errorf("invalid filename: %q", filename)
	}

	target, err := u.chats.UploadTarget(context.Background(), servicechat.ID(chatID))
	if err != nil {
		return fmt.Errorf("resolve chat %s upload dir: %w", chatID, err)
	}
	// Create the .uploads dir on demand, owned by container-root, with a
	// self-ignoring .gitignore so attachments never pollute the workspace repo.
	if err := os.MkdirAll(target, 0o755); err != nil {
		return fmt.Errorf("create uploads dir %s: %w", target, err)
	}
	_ = os.Chown(target, containerRootHostUID, containerRootHostUID)
	ensureUploadsGitignore(target)

	src := filepath.Join(u.tmpRoot, ev.Upload.ID)
	dest := filepath.Join(target, base)

	// Refuse to overwrite. The browser side disables the upload button when
	// the same name is already in the workspace, but races are possible.
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("destination exists: %s", dest)
	}

	// Rename if the temp file lives on the same filesystem as the workspace,
	// otherwise fall back to streaming copy (tmp is /var/lib/remote/uploads,
	// workspace is /var/lib/remote/projects — usually same mount).
	if err := os.Rename(src, dest); err != nil {
		if err := copyAndRemove(src, dest); err != nil {
			return fmt.Errorf("move upload: %w", err)
		}
	}

	if err := os.Chmod(dest, 0o644); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}
	if err := os.Chown(dest, containerRootHostUID, containerRootHostUID); err != nil {
		// Non-fatal — the file is there, just owned by host root. The
		// container's root may still read it depending on idmap mode.
		log.Printf("upload[%s] chown %s: %v", ev.Upload.ID, dest, err)
	}

	// tusd leaves a .info sidecar next to the file; clean it up.
	_ = os.Remove(src + ".info")

	log.Printf("upload[%s] %d B -> %s", ev.Upload.ID, ev.Upload.Size, dest)
	return nil
}

// ensureUploadsGitignore drops a self-ignoring .gitignore into the uploads
// dir so its contents never show up in the workspace repo's status.
func ensureUploadsGitignore(dir string) {
	p := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(p); err == nil {
		return
	}
	// "*" ignores every file here, including the .gitignore itself.
	if err := os.WriteFile(p, []byte("*\n"), 0o644); err != nil {
		log.Printf("uploads gitignore %s: %v", p, err)
		return
	}
	_ = os.Chown(p, containerRootHostUID, containerRootHostUID)
}

func copyAndRemove(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dest)
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}

// metaJSON is reserved for the frontend in case we want to surface upload
// metadata back to a poll endpoint. Unused for now.
var _ = json.Marshal
