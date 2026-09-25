package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// accountValidationTimeout bounds provider validation of a credential that
// no request context covers, such as the result of a finished login.
const accountValidationTimeout = 30 * time.Second

// accountErrorLimit bounds provider error text copied into login state.
const accountErrorLimit = 300

// AccountCredentials is the provider-owned half of saved accounts: where the
// CLI keeps its subscription login on the host, how the provider validates a
// credential, and which provider account a credential belongs to.
// AccountService treats the credential bytes as opaque.
type AccountCredentials interface {
	// ReadHost returns the login the host CLI currently uses, or an error
	// wrapping os.ErrNotExist when the host is signed out.
	ReadHost() (json.RawMessage, error)
	// WriteHost makes credential the host CLI's login.
	WriteHost(json.RawMessage) error
	// Validate asks the provider whether credential is a usable subscription
	// login. The returned credential may have been refreshed.
	Validate(context.Context, json.RawMessage) (ValidatedAccount, error)
	// Identity reads the stable account identifiers credential records.
	Identity(json.RawMessage) AccountIdentity
}

// AccountLoginFlow runs the provider's managed login for a saved account.
type AccountLoginFlow interface {
	// Reset makes way for a new account login: it stops a previous provider
	// login or reports that one is still running.
	Reset(context.Context) error
	// Prepare creates the isolated location one account login writes to.
	Prepare() (AccountLogin, error)
	// Start launches the provider login. The provider takes its CLI
	// environment from AccountService.LoginEnv and hands its outcome to
	// AccountService.FinishLogin.
	Start(context.Context) (LoginSnapshot, error)
}

// AccountLogin is one provider login running in an isolated location.
type AccountLogin interface {
	// Env points the provider CLI at the isolated location.
	Env(base []string) []string
	// Finish collects the credential the finished login wrote, then removes
	// the isolated location and undoes any write the CLI made outside it.
	// exitErr and output describe how the CLI ended.
	Finish(exitErr error, output string) (json.RawMessage, error)
	// Abort removes the isolated location without collecting a credential.
	Abort()
}

// AccountConfig describes one provider's saved accounts.
type AccountConfig struct {
	Provider agent.ProviderID
	// Label names the provider in user-facing errors, such as "Codex".
	Label       string
	Credentials AccountCredentials
	Login       AccountLoginFlow
	// LoginMayWriteHost declares a provider CLI that may write the host
	// credential instead of the isolated one during an account login. Runs
	// and account changes that write the host are then refused while an
	// account login is pending.
	LoginMayWriteHost bool
	// ReplacePendingLogin lets a new account login replace a pending one.
	// Use it when Reset stops the provider login without reporting an
	// outcome, which would otherwise leave the pending login forever.
	ReplacePendingLogin bool
	// Changed is called, without any account lock held, after an account
	// request that may have changed the account list.
	Changed func()
}

// AccountVault opens saved-account services over one persistence store.
// Provider modules receive the vault instead of the store, so the account
// lifecycle stays in this package and providers supply only credential
// mechanics.
type AccountVault struct {
	store AccountStore
}

// NewAccountVault returns nil when store is nil, which disables saved
// accounts.
func NewAccountVault(store AccountStore) *AccountVault {
	if store == nil {
		return nil
	}
	return &AccountVault{store: store}
}

// Open loads config.Provider's saved accounts and reconciles the host login
// with the committed active account.
func (v *AccountVault) Open(ctx context.Context, config AccountConfig) (*AccountService, error) {
	if config.Provider == "" || config.Label == "" || config.Credentials == nil || config.Login == nil {
		return nil, errors.New("saved accounts need a provider, label, credentials, and login flow")
	}
	accounts, err := v.store.AgentAccounts(ctx, config.Provider)
	if err != nil {
		return nil, err
	}
	service := &AccountService{config: config, store: v.store, accounts: accounts}
	if err := service.reconcileHost(); err != nil {
		log.Printf("%s accounts: %v", config.Provider, err)
	}
	return service, nil
}

// AccountService owns one provider's saved accounts: the committed account
// set, label and activation rules, account logins, isolated run snapshots and
// capture, and legacy run leases.
//
// The account store is the single authority for which account is active.
// Nothing changes when saving it fails; once it succeeds, memory follows it
// and the host login is brought in line. A host write that fails is not
// rolled back: the host is marked stale, the error is reported, and BeginRun
// writes the active account again before any run can use the old login.
type AccountService struct {
	config AccountConfig
	store  AccountStore

	// mutationMu serializes account changes, legacy run leases, isolated-run
	// captures, and login completion. Provider validation, which may take
	// seconds, runs under it.
	mutationMu sync.Mutex
	// mu guards the fields below. It is held only briefly so snapshots stay
	// responsive during validation.
	mu         sync.Mutex
	accounts   AccountSet
	login      *pendingAccountLogin
	activeRuns int
	hostStale  bool
	// isolatedRuns counts, per saved account, runs in progress in a private
	// home.
	isolatedRuns map[string]int
	usageReads   map[string]bool
	usageIdle    *sync.Cond
}

type pendingAccountLogin struct {
	accountID string
	label     string
	login     AccountLogin
}

// RunCredential is an immutable snapshot of one saved account handed to a
// provider adapter for an isolated run. Credential is never exposed through a
// transport; the adapter materializes it only in that run's private home.
type RunCredential struct {
	AccountID  string
	Credential json.RawMessage
}

var _ AccountController = (*AccountService)(nil)

func (s *AccountService) AccountsSnapshot() AccountsSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.accounts.Snapshot()
}

// CredentialForRun resolves accountID without changing the active account or
// canonical host login. Empty accountID selects the configured default. The
// bool is false only when no saved default exists, allowing legacy host login
// behavior to continue until an account is imported.
func (s *AccountService) CredentialForRun(accountID string) (RunCredential, bool, error) {
	accountID = strings.TrimSpace(accountID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if accountID == "" {
		accountID = s.accounts.ActiveAccountID
		if accountID == "" {
			return RunCredential{}, false, nil
		}
	}
	record, ok := s.accounts.Find(accountID)
	if !ok {
		return RunCredential{}, false, ErrAccountNotFound
	}
	return RunCredential{
		AccountID:  record.ID,
		Credential: append(json.RawMessage(nil), record.Credential...),
	}, true, nil
}

// CaptureRunCredential validates a credential refreshed in an isolated run
// and saves it back to the account that started the run. If that account was
// reconnected while the run was active, its newer saved credential wins and
// the stale run result is ignored.
func (s *AccountService) CaptureRunCredential(ctx context.Context, run RunCredential, credential json.RawMessage) error {
	credential = append(json.RawMessage(nil), credential...)
	if run.AccountID == "" || len(credential) == 0 || bytes.Equal(run.Credential, credential) {
		return nil
	}
	s.mutationMu.Lock()
	changed, err := s.captureRunCredentialLocked(ctx, run, credential)
	s.mutationMu.Unlock()
	if changed {
		s.changed()
	}
	return err
}

func (s *AccountService) captureRunCredentialLocked(ctx context.Context, run RunCredential, credential json.RawMessage) (bool, error) {
	s.mu.Lock()
	current, ok := s.accounts.Find(run.AccountID)
	s.mu.Unlock()
	if !ok {
		return false, ErrAccountNotFound
	}
	if !bytes.Equal(current.Credential, run.Credential) {
		return false, nil
	}
	if !s.sameAccount(run.Credential, credential) {
		return false, fmt.Errorf("%w: the refreshed %s login no longer belongs to saved account %q", ErrAccountIdentityMismatch, s.config.Label, current.Label)
	}
	validated, err := s.validate(ctx, credential)
	if err != nil {
		return false, fmt.Errorf("validate refreshed %s login: %w", s.config.Label, err)
	}
	if !s.sameAccount(run.Credential, validated.Credential) {
		return false, fmt.Errorf("%w: the validated %s login no longer belongs to saved account %q", ErrAccountIdentityMismatch, s.config.Label, current.Label)
	}

	s.mu.Lock()
	next := s.accounts.Clone()
	s.mu.Unlock()
	next.replace(newAccountRecord(run.AccountID, current.Label, validated))
	if err := s.commit(ctx, next, false); err != nil {
		return false, err
	}
	return true, nil
}

// ImportCurrent validates the current host login and saves it as a new
// active account.
func (s *AccountService) ImportCurrent(ctx context.Context, label string) error {
	label, err := normalizeAccountLabel(label)
	if err != nil {
		return err
	}
	credential, err := s.config.Credentials.ReadHost()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s is not signed in", s.config.Label)
		}
		return fmt.Errorf("read current %s credential: %w", s.config.Label, err)
	}
	defer s.changed()
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	s.mu.Lock()
	err = s.hostWritableLocked()
	next := s.accounts.Clone()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if err := next.ensureUniqueLabel(label, ""); err != nil {
		return err
	}
	validated, err := s.validate(ctx, credential)
	if err != nil {
		return err
	}
	id, err := newAccountID()
	if err != nil {
		return err
	}
	next.Accounts = append(next.Accounts, newAccountRecord(id, label, validated))
	next.ActiveAccountID = id
	return s.commit(ctx, next, true)
}

// StartAccountLogin starts a provider login for a new account, or for the
// saved account accountID. The finished login is saved by FinishLogin.
func (s *AccountService) StartAccountLogin(ctx context.Context, label, accountID string) (LoginSnapshot, error) {
	label, err := normalizeLoginLabel(label, accountID)
	if err != nil {
		return LoginSnapshot{}, err
	}
	if err := s.config.Login.Reset(ctx); err != nil {
		return LoginSnapshot{}, err
	}
	pending, err := s.beginLogin(label, accountID)
	if err != nil {
		return LoginSnapshot{}, err
	}
	snapshot, err := s.config.Login.Start(ctx)
	if err != nil {
		s.abortLogin(pending)
	}
	return snapshot, err
}

func (s *AccountService) beginLogin(label, accountID string) (*pendingAccountLogin, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeRuns > 0 {
		return nil, ErrAccountInUse
	}
	if s.login != nil {
		if !s.config.ReplacePendingLogin {
			return nil, fmt.Errorf("a %s %w", s.config.Label, ErrAccountLoginInProgress)
		}
		s.login.login.Abort()
		s.login = nil
	}
	if accountID != "" {
		record, ok := s.accounts.Find(accountID)
		if !ok {
			return nil, ErrAccountNotFound
		}
		if label == "" {
			label = record.Label
		}
	}
	if err := s.accounts.ensureUniqueLabel(label, accountID); err != nil {
		return nil, err
	}
	login, err := s.config.Login.Prepare()
	if err != nil {
		return nil, fmt.Errorf("prepare %s login: %w", s.config.Label, err)
	}
	s.login = &pendingAccountLogin{accountID: accountID, label: label, login: login}
	return s.login, nil
}

func (s *AccountService) abortLogin(pending *pendingAccountLogin) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	s.mu.Lock()
	current := s.login == pending
	if current {
		s.login = nil
	}
	s.mu.Unlock()
	if current {
		pending.login.Abort()
	}
}

// LoginEnv returns the provider CLI environment while an account login is
// pending, and false otherwise.
func (s *AccountService) LoginEnv(base []string) ([]string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.login == nil {
		return nil, false
	}
	return s.login.login.Env(base), true
}

// FinishLogin validates and saves what a pending account login wrote. The
// new account becomes active unless a run holds the current one. It reports
// handled=false when no account login is pending, leaving the outcome to the
// provider's plain login.
//
// The provider calls FinishLogin from its login completion, which then
// publishes the finished login state; FinishLogin therefore does not call
// Changed.
func (s *AccountService) FinishLogin(exitErr error, output string) (bool, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	s.mu.Lock()
	pending := s.login
	s.login = nil
	s.mu.Unlock()
	if pending == nil {
		return false, nil
	}

	credential, err := pending.login.Finish(exitErr, output)
	if err != nil {
		return true, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), accountValidationTimeout)
	defer cancel()
	validated, err := s.validate(ctx, credential)
	if err != nil {
		return true, shortError(err)
	}

	s.mu.Lock()
	next := s.accounts.Clone()
	activate := s.activeRuns == 0
	s.mu.Unlock()
	id := pending.accountID
	if id == "" {
		if id, err = newAccountID(); err != nil {
			return true, errors.New("could not create an account identifier")
		}
		next.Accounts = append(next.Accounts, newAccountRecord(id, pending.label, validated))
	} else {
		saved, ok := next.Find(id)
		if !ok {
			return true, ErrAccountNotFound
		}
		if !s.sameAccount(saved.Credential, validated.Credential) {
			return true, fmt.Errorf("%w: this login is not saved %s account %q; add it as a new account instead", ErrAccountIdentityMismatch, s.config.Label, saved.Label)
		}
		next.replace(newAccountRecord(id, pending.label, validated))
	}
	// Reconnecting the active account keeps its identity, so its new login
	// reaches the host even while a run holds the account.
	apply := activate || id == next.ActiveAccountID
	if activate {
		next.ActiveAccountID = id
	}
	return true, shortError(s.commit(ctx, next, apply))
}

// ActivateAccount validates the saved account accountID and makes it the
// host login.
func (s *AccountService) ActivateAccount(ctx context.Context, accountID string) error {
	defer s.changed()
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	s.mu.Lock()
	err := s.hostWritableLocked()
	_, ok := s.accounts.Find(accountID)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if !ok {
		return ErrAccountNotFound
	}
	// The active account's host login may be newer than its saved copy;
	// keep it before activation replaces it.
	if err := s.captureLocked(ctx); err != nil {
		log.Printf("%s accounts: keep the active account's login: %v", s.config.Provider, err)
	}
	s.mu.Lock()
	record, _ := s.accounts.Find(accountID)
	s.mu.Unlock()
	validated, err := s.validate(ctx, record.Credential)
	if err != nil {
		return err
	}
	if !s.sameAccount(record.Credential, validated.Credential) {
		return fmt.Errorf("%w: the saved %s login %q now signs in as another account; reconnect it", ErrAccountIdentityMismatch, s.config.Label, record.Label)
	}

	s.mu.Lock()
	next := s.accounts.Clone()
	s.mu.Unlock()
	next.replace(newAccountRecord(accountID, record.Label, validated))
	next.ActiveAccountID = accountID
	return s.commit(ctx, next, true)
}

// DeleteAccount removes a saved account other than the active one.
func (s *AccountService) DeleteAccount(ctx context.Context, accountID string) error {
	defer s.changed()
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	s.mu.Lock()
	active := accountID == s.accounts.ActiveAccountID
	next := s.accounts.Clone()
	s.mu.Unlock()
	if active {
		return ErrActiveAccountDelete
	}
	if !next.remove(accountID) {
		return ErrAccountNotFound
	}
	return s.commit(ctx, next, false)
}

// BeginRun leases the active account for one provider run so it cannot be
// switched underneath the CLI. The host login is first brought in line with
// the active account: a login left stale by a failed write, a signed-out
// host, or a login for another account is replaced. The run is refused while
// that write keeps failing.
func (s *AccountService) BeginRun() (func(), error) {
	return s.BeginRunFor(context.Background(), "")
}

// BeginRunFor leases accountID for one provider run. Selecting a different
// saved account is atomic with taking the lease, so another run cannot start
// between applying that account to the host and the caller launching its CLI.
// The selected account becomes the provider's active account (the most recent
// run selection) and is published to auth subscribers. Empty accountID keeps
// the current active account.
func (s *AccountService) BeginRunFor(ctx context.Context, accountID string) (func(), error) {
	accountID = strings.TrimSpace(accountID)
	s.mutationMu.Lock()
	release, changed, err := s.beginRunLocked(ctx, accountID)
	s.mutationMu.Unlock()
	if changed {
		s.changed()
	}
	return release, err
}

func (s *AccountService) beginRunLocked(ctx context.Context, accountID string) (func(), bool, error) {
	s.mu.Lock()
	if s.config.LoginMayWriteHost && s.login != nil {
		s.mu.Unlock()
		return nil, false, fmt.Errorf("%s %w", s.config.Label, ErrAccountLoginInProgress)
	}
	activeAccountID := s.accounts.ActiveAccountID
	if accountID == "" || accountID == activeAccountID {
		if err := s.reconcileHostLocked(); err != nil {
			s.mu.Unlock()
			return nil, false, err
		}
		release := s.leaseRunLocked()
		s.mu.Unlock()
		return release, false, nil
	}
	if s.activeRuns > 0 {
		s.mu.Unlock()
		return nil, false, ErrAccountInUse
	}
	record, ok := s.accounts.Find(accountID)
	s.mu.Unlock()
	if !ok {
		return nil, false, ErrAccountNotFound
	}

	// Preserve a refresh made by the outgoing account before replacing the
	// canonical host credential. As with explicit activation, a transient
	// capture failure does not make a valid saved target unusable.
	if err := s.captureLocked(ctx); err != nil {
		log.Printf("%s accounts: keep the active account's login before run: %v", s.config.Provider, err)
	}
	validated, err := s.validate(ctx, record.Credential)
	if err != nil {
		return nil, false, err
	}
	if !s.sameAccount(record.Credential, validated.Credential) {
		return nil, false, fmt.Errorf("%w: the saved %s login %q now signs in as another account; reconnect it", ErrAccountIdentityMismatch, s.config.Label, record.Label)
	}

	s.mu.Lock()
	next := s.accounts.Clone()
	s.mu.Unlock()
	next.replace(newAccountRecord(accountID, record.Label, validated))
	next.ActiveAccountID = accountID
	if err := s.commit(ctx, next, true); err != nil {
		return nil, false, err
	}

	s.mu.Lock()
	release := s.leaseRunLocked()
	s.mu.Unlock()
	return release, true, nil
}

// leaseRunLocked increments the lease count and returns an idempotent release.
// s.mu must be held by the caller.
func (s *AccountService) leaseRunLocked() func() {
	s.activeRuns++
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			s.activeRuns--
			s.mu.Unlock()
		})
	}
}

// BeginIsolatedRun records a run of accountID in a private home until the
// returned release is called. Plan-usage reads leave such an account alone:
// refreshing its login could spend the refresh token the running chat still
// holds.
func (s *AccountService) BeginIsolatedRun(accountID string) func() {
	s.mu.Lock()
	if s.usageIdle == nil {
		s.usageIdle = sync.NewCond(&s.mu)
	}
	for s.usageReads[accountID] {
		s.usageIdle.Wait()
	}
	if s.isolatedRuns == nil {
		s.isolatedRuns = make(map[string]int)
	}
	s.isolatedRuns[accountID]++
	s.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			s.isolatedRuns[accountID]--
			if s.isolatedRuns[accountID] <= 0 {
				delete(s.isolatedRuns, accountID)
			}
			s.mu.Unlock()
		})
	}
}

// WithIdleIsolatedAccount reads one saved account only while no run uses its
// credential. A run starting during the read waits until read has offered any
// refreshed login back to the vault.
func (s *AccountService) WithIdleIsolatedAccount(accountID string, read func()) bool {
	s.mu.Lock()
	if s.isolatedRuns[accountID] > 0 || s.usageReads[accountID] {
		s.mu.Unlock()
		return false
	}
	if s.usageReads == nil {
		s.usageReads = make(map[string]bool)
	}
	s.usageReads[accountID] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.usageReads, accountID)
		if s.usageIdle != nil {
			s.usageIdle.Broadcast()
		}
		s.mu.Unlock()
	}()
	read()
	return true
}

// WithIdleHostLogin runs read against the host login while no account change
// can rewrite it: activations, imports, and finished logins wait until read
// returns. It returns false without running read while a legacy run, or an
// account login that may write the host, is using the host login.
func (s *AccountService) WithIdleHostLogin(expectedActiveAccountID string, read func()) bool {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	s.mu.Lock()
	busy := s.activeRuns > 0 || (s.config.LoginMayWriteHost && s.login != nil) ||
		s.accounts.ActiveAccountID != expectedActiveAccountID ||
		(expectedActiveAccountID != "" && (s.isolatedRuns[expectedActiveAccountID] > 0 || s.usageReads[expectedActiveAccountID]))
	if busy {
		s.mu.Unlock()
		return false
	}
	if expectedActiveAccountID != "" {
		if s.usageReads == nil {
			s.usageReads = make(map[string]bool)
		}
		s.usageReads[expectedActiveAccountID] = true
		defer func() {
			s.mu.Lock()
			delete(s.usageReads, expectedActiveAccountID)
			if s.usageIdle != nil {
				s.usageIdle.Broadcast()
			}
			s.mu.Unlock()
		}()
	}
	s.mu.Unlock()
	read()
	return true
}

// CaptureAfterRun keeps a login the provider CLI refreshed during a run.
// Call it after a successful run has synchronized credentials back from its
// project container.
func (s *AccountService) CaptureAfterRun(ctx context.Context) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	return s.captureLocked(ctx)
}

// captureLocked saves the host login into the active account when it
// changed. A run may just have copied that login back from an untrusted
// project container, so it is saved only after the provider validates it
// and only while it still signs in as the active account. A login for any
// other account is replaced with the saved one, so it neither enters the
// vault nor reaches other containers. A login that fails validation is left
// on the host but not saved, because the failure may be transient and the
// saved copy may hold a refresh token the CLI has since rotated.
func (s *AccountService) captureLocked(ctx context.Context) error {
	s.mu.Lock()
	active, ok := s.accounts.Find(s.accounts.ActiveAccountID)
	stale := s.hostStale
	s.mu.Unlock()
	if !ok {
		return nil
	}
	if stale {
		// The host has not received the committed login yet, so what it
		// holds is older, not refreshed.
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.applyLocked()
	}
	current, err := s.config.Credentials.ReadHost()
	if err != nil {
		return fmt.Errorf("read %s credential: %w", s.config.Label, err)
	}
	if bytes.Equal(current, active.Credential) {
		return nil
	}
	if !s.sameAccount(active.Credential, current) {
		return s.restoreActive(active)
	}
	validated, err := s.validate(ctx, current)
	if err != nil {
		return fmt.Errorf("validate refreshed %s login: %w", s.config.Label, err)
	}
	if !s.sameAccount(active.Credential, validated.Credential) {
		return s.restoreActive(active)
	}

	s.mu.Lock()
	next := s.accounts.Clone()
	s.mu.Unlock()
	next.replace(newAccountRecord(active.ID, active.Label, validated))
	return s.commit(ctx, next, !bytes.Equal(validated.Credential, current))
}

// restoreActive replaces a host login that does not belong to the active
// account with the active account's saved login.
func (s *AccountService) restoreActive(active AccountRecord) error {
	mismatch := fmt.Errorf("%w: the host %s login is not saved account %q, which was restored", ErrAccountIdentityMismatch, s.config.Label, active.Label)
	s.mu.Lock()
	defer s.mu.Unlock()
	return errors.Join(mismatch, s.applyLocked())
}

// reconcileHost makes the host login match the committed active account
// when the service opens.
func (s *AccountService) reconcileHost() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reconcileHostLocked()
}

// reconcileHostLocked keeps a host login for the active account, because
// the CLI may have refreshed it after the saved copy was taken; the next
// capture validates and saves it. A stale, missing, or foreign host login is
// replaced with the active account's saved login.
func (s *AccountService) reconcileHostLocked() error {
	active, ok := s.accounts.Find(s.accounts.ActiveAccountID)
	if !ok {
		return nil
	}
	if !s.hostStale {
		if current, err := s.config.Credentials.ReadHost(); err == nil && s.sameAccount(active.Credential, current) {
			return nil
		}
	}
	return s.applyLocked()
}

// hostWritableLocked refuses an account change that writes the host while a
// run holds the account or while a login may write the host itself.
func (s *AccountService) hostWritableLocked() error {
	if s.activeRuns > 0 {
		return ErrAccountInUse
	}
	if s.config.LoginMayWriteHost && s.login != nil {
		return fmt.Errorf("a %s %w", s.config.Label, ErrAccountLoginInProgress)
	}
	return nil
}

// commit saves next as the account set and, with apply, writes its active
// account to the host.
func (s *AccountService) commit(ctx context.Context, next AccountSet, apply bool) error {
	if err := s.store.SaveAgentAccounts(ctx, s.config.Provider, next); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accounts = next
	if !apply {
		return nil
	}
	return s.applyLocked()
}

// applyLocked writes the committed active account to the host and records
// whether the host is stale.
func (s *AccountService) applyLocked() error {
	active, ok := s.accounts.Find(s.accounts.ActiveAccountID)
	if !ok {
		s.hostStale = false
		return nil
	}
	if err := s.config.Credentials.WriteHost(active.Credential); err != nil {
		s.hostStale = true
		return fmt.Errorf("%s account %q is selected but %w: %v; Remote retries before the next run", s.config.Label, active.Label, ErrAccountNotApplied, err)
	}
	s.hostStale = false
	return nil
}

// validate runs provider validation and requires the validated login to
// identify its account, which later captures and activations compare against.
func (s *AccountService) validate(ctx context.Context, credential json.RawMessage) (ValidatedAccount, error) {
	validated, err := s.config.Credentials.Validate(ctx, credential)
	if err != nil {
		return ValidatedAccount{}, err
	}
	if !s.config.Credentials.Identity(validated.Credential).Known() {
		return ValidatedAccount{}, fmt.Errorf("could not tell which %s account this login belongs to", s.config.Label)
	}
	return validated, nil
}

func (s *AccountService) sameAccount(saved, candidate json.RawMessage) bool {
	return s.config.Credentials.Identity(saved).Same(s.config.Credentials.Identity(candidate))
}

func (s *AccountService) changed() {
	if s.config.Changed != nil {
		s.config.Changed()
	}
}

func newAccountRecord(id, label string, validated ValidatedAccount) AccountRecord {
	return AccountRecord{
		ID: id, Label: label, Email: validated.Email, PlanType: validated.PlanType,
		ValidatedAt: time.Now().UTC(),
		Credential:  append(json.RawMessage(nil), validated.Credential...),
	}
}

// shortError bounds provider error text copied into login state.
func shortError(err error) error {
	if err == nil || len(err.Error()) <= accountErrorLimit {
		return err
	}
	return errors.New(err.Error()[:accountErrorLimit] + "...")
}
