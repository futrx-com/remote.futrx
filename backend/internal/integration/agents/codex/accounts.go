package codex

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

// accountCredentials is Codex's half of saved accounts: the host auth.json
// the CLI reads, validation through the Codex app server, and the ChatGPT
// account a login names.
type accountCredentials struct {
	// validate asks Codex whether a credential is a usable ChatGPT login.
	// Tests replace it because the real check reaches OpenAI.
	validate func(context.Context, json.RawMessage) (agentauth.ValidatedAccount, error)
}

var _ agentauth.AccountCredentials = (*accountCredentials)(nil)

func (c *accountCredentials) ReadHost() (json.RawMessage, error) {
	return os.ReadFile(codexCredentialPath())
}

func (c *accountCredentials) WriteHost(credential json.RawMessage) error {
	return agentauth.WriteCredentialFile(codexCredentialPath(), credential)
}

func (c *accountCredentials) Validate(ctx context.Context, credential json.RawMessage) (agentauth.ValidatedAccount, error) {
	return c.validate(ctx, credential)
}

func (c *accountCredentials) Identity(credential json.RawMessage) agentauth.AccountIdentity {
	return codexAccountIdentity(credential)
}

// codexIDTokenClaims are the id_token claims that name a ChatGPT account.
type codexIDTokenClaims struct {
	Email string `json:"email"`
	Auth  struct {
		ChatGPTAccountID string `json:"chatgpt_account_id"`
		ChatGPTUserID    string `json:"chatgpt_user_id"`
		UserID           string `json:"user_id"`
	} `json:"https://api.openai.com/auth"`
}

// codexAccountIdentity reads which ChatGPT account and user a Codex auth.json
// signs in as. The id_token signature is not verified: identity only checks
// that a login still belongs to its saved account, and a login is saved only
// after Validate has refreshed it through the Codex app server. An API-key or
// unreadable login names no account.
func codexAccountIdentity(credential json.RawMessage) agentauth.AccountIdentity {
	var file struct {
		AuthMode string `json:"auth_mode"`
		Tokens   *struct {
			IDToken   string `json:"id_token"`
			AccountID string `json:"account_id"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(credential, &file); err != nil || file.Tokens == nil ||
		strings.EqualFold(strings.TrimSpace(file.AuthMode), "apikey") {
		return nil
	}
	claims := parseCodexIDToken(file.Tokens.IDToken)
	identity := agentauth.AccountIdentity{}
	// set records the first non-empty candidate for key.
	set := func(key string, candidates ...string) {
		for _, value := range candidates {
			if value = strings.TrimSpace(value); value != "" {
				identity[key] = value
				return
			}
		}
	}
	set("account", file.Tokens.AccountID, claims.Auth.ChatGPTAccountID)
	// The user ID names the person within a possibly shared workspace
	// account. The email can change, so it stands in only without one.
	set("user", claims.Auth.ChatGPTUserID, claims.Auth.UserID)
	if identity["user"] == "" {
		set("email", strings.ToLower(claims.Email))
	}
	return identity
}

// parseCodexIDToken decodes the claims of a JWT without verifying it. A
// malformed token yields no claims.
func parseCodexIDToken(token string) codexIDTokenClaims {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return codexIDTokenClaims{}
	}
	// JWTs omit base64 padding, but tolerate a token that kept it.
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return codexIDTokenClaims{}
	}
	var claims codexIDTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return codexIDTokenClaims{}
	}
	return claims
}

// accountLoginFlow runs `codex login --device-auth` for a saved account.
type accountLoginFlow struct{ auth *Auth }

var _ agentauth.AccountLoginFlow = accountLoginFlow{}

// Reset refuses to start while any Codex login runs: the device login is
// shared by plain and account logins.
func (f accountLoginFlow) Reset(context.Context) error {
	if f.auth.device.LoginState().Active {
		return fmt.Errorf("a Codex %w", agentauth.ErrAccountLoginInProgress)
	}
	return nil
}

func (f accountLoginFlow) Prepare() (agentauth.AccountLogin, error) {
	login, err := newAccountLogin(externalCredentialPaths())
	if err != nil {
		return nil, err
	}
	return login, nil
}

func (f accountLoginFlow) Start(ctx context.Context) (agentauth.LoginSnapshot, error) {
	state, err := f.auth.device.StartDeviceLogin(ctx)
	return agentauth.LoginSnapshot{
		Active: state.Active, URL: state.VerificationURI, UserCode: state.UserCode,
		StartedAt: state.StartedAt, ExpiresAt: state.ExpiresAt,
		Completed: state.Completed, Error: state.Error,
	}, err
}

// accountLogin is one Codex account login writing to an isolated CODEX_HOME.
type accountLogin struct {
	root string
	home string
	// external holds host credential files a Codex build that ignores the
	// isolated CODEX_HOME writes instead, such as a Snap package that pins
	// CODEX_HOME to its own data directory.
	external []watchedCredential
}

var _ agentauth.AccountLogin = (*accountLogin)(nil)

// newAccountLogin creates the isolated CODEX_HOME and remembers each host
// credential file in externalPaths so a write to it can be undone.
func newAccountLogin(externalPaths []string) (*accountLogin, error) {
	root, home, err := prepareIsolatedCodexHome("remote-codex-login-*")
	if err != nil {
		return nil, err
	}
	login := &accountLogin{root: root, home: home, external: make([]watchedCredential, 0, len(externalPaths))}
	for _, path := range externalPaths {
		watched, err := watchCredential(path)
		if err != nil {
			_ = os.RemoveAll(root)
			return nil, fmt.Errorf("read current Codex credential: %w", err)
		}
		login.external = append(login.external, watched)
	}
	return login, nil
}

func (l *accountLogin) Env(base []string) []string {
	return isolatedCodexAuthEnvFor(base, l.home)
}

// Finish takes the login's credential, then puts every host file the CLI
// touched back as it was. Saving the account decides what becomes active.
func (l *accountLogin) Finish(exitErr error, output string) (json.RawMessage, error) {
	defer os.RemoveAll(l.root)
	credential, readErr := l.credential()
	if err := l.restoreExternal(); err != nil {
		return nil, errors.New("restore previous Codex credential: " + truncate(err.Error(), 160))
	}
	if exitErr != nil {
		return nil, accountLoginFailure(fmt.Sprintf("codex login failed: %s", truncate(exitErr.Error(), 300)), output)
	}
	if readErr != nil {
		return nil, accountLoginFailure(truncate(readErr.Error(), 400), output)
	}
	return credential, nil
}

// Abort undoes what a login that will not finish may already have written.
func (l *accountLogin) Abort() {
	_ = l.restoreExternal()
	_ = os.RemoveAll(l.root)
}

// credential returns what the login wrote: the isolated CODEX_HOME file when
// the CLI honored it, otherwise a host file it changed instead.
func (l *accountLogin) credential() (json.RawMessage, error) {
	isolatedPath := filepath.Join(l.home, "auth.json")
	credential, err := os.ReadFile(isolatedPath)
	if err == nil {
		return credential, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read Codex login credential: %w", err)
	}
	checked := []string{isolatedPath}
	for _, watched := range l.external {
		if current, changed := watched.changed(); changed {
			return current, nil
		}
		checked = append(checked, watched.path)
	}
	return nil, fmt.Errorf("Codex login completed without writing credentials (checked %s)", strings.Join(checked, ", "))
}

func (l *accountLogin) restoreExternal() error {
	var failures []error
	for _, watched := range l.external {
		if _, changed := watched.changed(); !changed {
			continue
		}
		if err := watched.restore(); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// accountLoginFailure keeps the CLI's own last words, which usually say why
// no credential was written.
func accountLoginFailure(message, output string) error {
	if output != "" {
		message += "; codex output: " + output
	}
	return errors.New(message)
}

// watchedCredential remembers a host credential file as it was before an
// account login so the login's write can be detected and undone.
type watchedCredential struct {
	path     string
	previous []byte
	existed  bool
}

func watchCredential(path string) (watchedCredential, error) {
	previous, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return watchedCredential{}, err
	}
	return watchedCredential{path: path, previous: previous, existed: err == nil}, nil
}

// changed returns the file's current content when the login wrote to it.
func (w watchedCredential) changed() ([]byte, bool) {
	current, err := os.ReadFile(w.path)
	if err != nil {
		return nil, false
	}
	return current, !w.existed || !bytes.Equal(current, w.previous)
}

func (w watchedCredential) restore() error {
	if w.existed {
		return agentauth.WriteCredentialFile(w.path, w.previous)
	}
	err := os.Remove(w.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func prepareIsolatedCodexHome(pattern string) (string, string, error) {
	root, err := os.MkdirTemp("", pattern)
	if err != nil {
		return "", "", err
	}
	if err := os.Chmod(root, 0o700); err != nil {
		_ = os.RemoveAll(root)
		return "", "", err
	}
	home := filepath.Join(root, ".codex")
	if err := os.Mkdir(home, 0o700); err != nil {
		_ = os.RemoveAll(root)
		return "", "", err
	}
	return root, home, nil
}
