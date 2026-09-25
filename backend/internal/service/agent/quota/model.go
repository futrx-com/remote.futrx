package quota

import "github.com/futrx-com/remote.futrx.com/internal/agent"

// AccountQuota is every window one provider account has reported.
type AccountQuota struct {
	Provider string `json:"provider"`
	// AccountID is the saved account whose plan the windows describe. It is
	// empty for the provider's host login, which runs without a pinned
	// account use only while the provider has no active saved account.
	AccountID string `json:"accountId,omitempty"`
	// Session and Weekly are pointers because "not reported" and "reported
	// as empty" are different states and only one of them should render.
	Session *agent.Quota `json:"session,omitempty"`
	Weekly  *agent.Quota `json:"weekly,omitempty"`
}

// accountKey names one provider account's plan.
type accountKey struct {
	provider string
	account  string
}

func (q AccountQuota) key() accountKey {
	return accountKey{provider: q.Provider, account: q.AccountID}
}

func (q AccountQuota) clone() AccountQuota {
	q.Session = cloneWindow(q.Session)
	q.Weekly = cloneWindow(q.Weekly)
	return q
}

func cloneWindow(window *agent.Quota) *agent.Quota {
	if window == nil {
		return nil
	}
	copy := *window
	if window.UsedPercent != nil {
		used := *window.UsedPercent
		copy.UsedPercent = &used
	}
	return &copy
}
