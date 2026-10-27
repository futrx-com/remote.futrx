package project

import (
	"context"
	"errors"
	"log"
	"strings"
)

// accessList owns per-project membership. Emails are normalized to lowercase
// here, at the one boundary that reads and writes them, so a stored entry and
// a lookup can never disagree on their form. A nil AccessRepository means
// membership is not enforced and every project is visible to everyone.
type accessList struct {
	repo AccessRepository
}

func newAccessList(repo AccessRepository) *accessList {
	return &accessList{repo: repo}
}

func (a *accessList) has(ctx context.Context, id ID, email string) (bool, error) {
	if a.repo == nil {
		return false, nil
	}
	em := normalizeEmail(email)
	if em == "" {
		return false, nil
	}
	return a.repo.Has(ctx, id, em)
}

func (a *accessList) list(ctx context.Context, id ID) ([]string, error) {
	if a.repo == nil {
		return nil, nil
	}
	return a.repo.List(ctx, id)
}

func (a *accessList) add(ctx context.Context, id ID, email string) error {
	if a.repo == nil {
		return errors.New("access store unavailable")
	}
	em := normalizeEmail(email)
	if em == "" {
		return errors.New("empty email")
	}
	return a.repo.Add(ctx, id, em)
}

func (a *accessList) remove(ctx context.Context, id ID, email string) error {
	if a.repo == nil {
		return nil
	}
	em := normalizeEmail(email)
	if em == "" {
		return nil
	}
	return a.repo.Remove(ctx, id, em)
}

func (a *accessList) count(ctx context.Context, id ID) (int, error) {
	if a.repo == nil {
		return 0, nil
	}
	list, err := a.repo.List(ctx, id)
	if err != nil {
		return 0, err
	}
	return len(list), nil
}

func (a *accessList) deleteAll(ctx context.Context, id ID) error {
	if a.repo == nil {
		return nil
	}
	return a.repo.DeleteAll(ctx, id)
}

// seed grants the project's creator access. Best-effort: a project that
// outlives this failure is still reachable by admins.
func (a *accessList) seed(ctx context.Context, id ID, email string) {
	if a.repo == nil {
		return
	}
	em := normalizeEmail(email)
	if em == "" {
		return
	}
	if err := a.repo.Add(ctx, id, em); err != nil {
		log.Printf("projects: seed access for %s: %v", id, err)
	}
}

// filterVisible keeps only the projects callerEmail is a member of. An
// unreadable membership list hides that project rather than failing the whole
// listing. Admin bypass is the caller's policy, not this list's.
func (a *accessList) filterVisible(ctx context.Context, all []Meta, callerEmail string) []Meta {
	if a.repo == nil {
		return all
	}
	em := normalizeEmail(callerEmail)
	if em == "" {
		return nil
	}
	out := make([]Meta, 0, len(all))
	for _, m := range all {
		ok, err := a.repo.Has(ctx, m.ID, em)
		if err != nil {
			log.Printf("projects: access check %s/%s: %v", m.ID, em, err)
			continue
		}
		if ok {
			out = append(out, m)
		}
	}
	return out
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
