package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

// accountCredentialKeys are the .credentials.json entries that belong to one
// Claude subscription login. Other entries, such as MCP server OAuth tokens,
// stay with the host and survive an account switch.
var accountCredentialKeys = []string{"claudeAiOauth", "organizationUuid"}

// writeCredentialFile replaces one CLI file atomically. Tests replace it to
// provoke write failures that file permissions cannot, since they may run as
// root.
var writeCredentialFile = agentauth.WriteCredentialFile

// accountCredential is the opaque value saved per account: the account's
// token entries plus the oauthAccount profile the CLI keeps in .claude.json.
type accountCredential struct {
	Credentials  map[string]json.RawMessage `json:"credentials"`
	OAuthAccount json.RawMessage            `json:"oauthAccount,omitempty"`
}

// accountCredentials is Claude's half of saved accounts: the host login lives
// in two CLI files, the CLI validates it, and oauthAccount identifies it.
type accountCredentials struct {
	// validate is validateAccountCredential; tests replace it.
	validate func(context.Context, json.RawMessage) (agentauth.ValidatedAccount, error)
}

var _ agentauth.AccountCredentials = (*accountCredentials)(nil)

func (c *accountCredentials) ReadHost() (json.RawMessage, error) {
	return readAccountCredential(claudeCredentialPath(), claudeGlobalConfigPath())
}

func (c *accountCredentials) WriteHost(credential json.RawMessage) error {
	return writeAccountCredential(claudeCredentialPath(), claudeGlobalConfigPath(), credential)
}

func (c *accountCredentials) Validate(ctx context.Context, credential json.RawMessage) (agentauth.ValidatedAccount, error) {
	return c.validate(ctx, credential)
}

func (c *accountCredentials) Identity(credential json.RawMessage) agentauth.AccountIdentity {
	return accountIdentity(credential)
}

// accountIdentity reads the account a saved credential signs in as from its
// oauthAccount profile: the stable account UUID and the email address.
func accountIdentity(credential []byte) agentauth.AccountIdentity {
	identity := agentauth.AccountIdentity{}
	var stored accountCredential
	if json.Unmarshal(credential, &stored) != nil || len(stored.OAuthAccount) == 0 {
		return identity
	}
	var profile struct {
		AccountUUID  string `json:"accountUuid"`
		EmailAddress string `json:"emailAddress"`
	}
	if json.Unmarshal(stored.OAuthAccount, &profile) != nil {
		return identity
	}
	// The account UUID is stable; the email can change, so it stands in only
	// for a profile without a UUID.
	if account := strings.TrimSpace(profile.AccountUUID); account != "" {
		identity["account"] = account
	} else if email := strings.ToLower(strings.TrimSpace(profile.EmailAddress)); email != "" {
		identity["email"] = email
	}
	return identity
}

// accountLoginFlow runs `claude auth login` for a saved account through the
// shared code service. The pasted code arrives through the regular code
// route, and the code service's completion hands the result to
// AccountService.FinishLogin.
type accountLoginFlow struct{ auth *Auth }

var _ agentauth.AccountLoginFlow = accountLoginFlow{}

// Reset stops a code login that was abandoned or timed out, which never
// resolves on its own, so a new account login replaces it instead of waiting
// for it.
func (f accountLoginFlow) Reset(ctx context.Context) error {
	return f.auth.code.Cancel(ctx)
}

func (f accountLoginFlow) Prepare() (agentauth.AccountLogin, error) {
	root, home, err := prepareIsolatedClaudeHome("remote-claude-login-*")
	if err != nil {
		return nil, err
	}
	return &accountLogin{root: root, home: home}, nil
}

func (f accountLoginFlow) Start(ctx context.Context) (agentauth.LoginSnapshot, error) {
	if _, err := f.auth.code.Start(ctx); err != nil {
		return agentauth.LoginSnapshot{}, err
	}
	state := f.auth.code.Status().Login
	return agentauth.LoginSnapshot{
		Active: state.Active, URL: state.AuthURL, AwaitingCode: state.AwaitingCode,
		StartedAt: state.StartedAt, Completed: state.Completed, Error: state.Error,
	}, nil
}

// accountLogin is one `claude auth login` writing to a private
// CLAUDE_CONFIG_DIR, so the host login is untouched until AccountService has
// validated and saved the result.
type accountLogin struct {
	root string
	home string
}

var _ agentauth.AccountLogin = (*accountLogin)(nil)

func (l *accountLogin) Env(base []string) []string {
	return isolatedClaudeAuthEnvFor(base, l.home)
}

// Finish reads the login the CLI wrote to the private directory.
func (l *accountLogin) Finish(exitErr error, output string) (json.RawMessage, error) {
	defer l.Abort()
	if exitErr != nil && !errors.Is(exitErr, context.Canceled) {
		return nil, accountLoginFailure(fmt.Sprintf("claude login failed: %s", truncate(exitErr.Error(), 300)), output)
	}
	credentialPath := filepath.Join(l.home, ".credentials.json")
	credential, err := readAccountCredential(credentialPath, filepath.Join(l.home, ".claude.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, accountLoginFailure("Claude login completed without writing credentials to "+credentialPath, output)
	}
	if err != nil {
		return nil, accountLoginFailure("read Claude login credential: "+truncate(err.Error(), 300), output)
	}
	return credential, nil
}

func (l *accountLogin) Abort() {
	_ = os.RemoveAll(l.root)
}

// accountLoginFailure keeps the CLI's own last words, which usually say why
// no credential was written.
func accountLoginFailure(message, output string) error {
	if output != "" {
		message += "; claude output: " + output
	}
	return errors.New(message)
}

func prepareIsolatedClaudeHome(pattern string) (string, string, error) {
	root, err := os.MkdirTemp("", pattern)
	if err != nil {
		return "", "", err
	}
	if err := os.Chmod(root, 0o700); err != nil {
		_ = os.RemoveAll(root)
		return "", "", err
	}
	home := filepath.Join(root, ".claude")
	if err := os.Mkdir(home, 0o700); err != nil {
		_ = os.RemoveAll(root)
		return "", "", err
	}
	return root, home, nil
}

// readAccountCredential extracts one account's entries from the CLI's
// credential and global config files.
func readAccountCredential(credentialPath, configPath string) (json.RawMessage, error) {
	credentials, err := readJSONObject(credentialPath)
	if err != nil {
		return nil, err
	}
	if len(credentials["claudeAiOauth"]) == 0 {
		return nil, fmt.Errorf("%s has no Claude subscription login: %w", credentialPath, os.ErrNotExist)
	}
	stored := accountCredential{Credentials: map[string]json.RawMessage{}}
	for _, key := range accountCredentialKeys {
		if value, ok := credentials[key]; ok {
			stored.Credentials[key] = value
		}
	}
	config, err := readJSONObject(configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	stored.OAuthAccount = config["oauthAccount"]
	return json.Marshal(stored)
}

// writeAccountCredential merges one account into the CLI's files, keeping
// every unrelated entry. If the second file cannot be written, the first is
// restored so the host never mixes two accounts; a failed restore is
// reported with the write error, because the host may then do exactly that.
func writeAccountCredential(credentialPath, configPath string, credential []byte) error {
	var stored accountCredential
	if err := json.Unmarshal(credential, &stored); err != nil || len(stored.Credentials["claudeAiOauth"]) == 0 {
		return errors.New("saved Claude credential is not a Claude subscription login")
	}
	previousCredentials, readErr := os.ReadFile(credentialPath)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	credentials, err := readJSONObject(credentialPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if credentials == nil {
		credentials = map[string]json.RawMessage{}
	}
	for _, key := range accountCredentialKeys {
		delete(credentials, key)
		if value, ok := stored.Credentials[key]; ok {
			credentials[key] = value
		}
	}
	config, err := readJSONObject(configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if config == nil {
		config = map[string]json.RawMessage{}
	}
	delete(config, "oauthAccount")
	if len(stored.OAuthAccount) > 0 {
		config["oauthAccount"] = stored.OAuthAccount
	}

	if err := writeJSONObject(credentialPath, credentials); err != nil {
		return err
	}
	if err := writeJSONObject(configPath, config); err != nil {
		var restoreErr error
		if readErr == nil {
			restoreErr = writeCredentialFile(credentialPath, previousCredentials)
		} else {
			restoreErr = os.Remove(credentialPath)
		}
		if restoreErr != nil {
			return errors.Join(err, fmt.Errorf("restore previous Claude credentials: %w", restoreErr))
		}
		return err
	}
	return nil
}

func readJSONObject(path string) (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if object == nil {
		object = map[string]json.RawMessage{}
	}
	return object, nil
}

func writeJSONObject(path string, object map[string]json.RawMessage) error {
	data, err := json.MarshalIndent(object, "", "  ")
	if err != nil {
		return err
	}
	return writeCredentialFile(path, data)
}
