package provider

import "errors"

// ErrNoReceipt reports that a provider ran successfully but had nothing to
// download this time — e.g. an email-based provider (Yettel, eUpravnik) whose
// invoice email has not arrived in the mailbox yet. It is a normal outcome, not
// a failure: the refresh runner treats it as "nothing to fetch" so the UI can
// explain why a provider produced no receipt without flagging an error.
var ErrNoReceipt = errors.New("no receipt available to download")

type Interface interface {
	DownloadReceipt() error
}

// Job pairs a provider implementation with the identity of the account it runs
// for, so a refresh over several accounts can report each outcome against the
// right provider and account. Account is "" for a solo deployment; Label is the
// human-friendly account name for notifications and the dashboard (falls back to
// the account id, then to nothing, when unset).
type Job struct {
	Impl     Interface
	Provider string
	Account  string
	Label    string
}

// Key is the stable identifier for a job within a refresh run: the provider for
// a solo account, or "provider\x00account" for a named one. It is used as the
// map key so per-account jobs never collide.
func (j Job) Key() string {
	return JobKey(j.Provider, j.Account)
}

// JobKey builds the per-account job key (see Job.Key).
func JobKey(provider, account string) string {
	if account == "" {
		return provider
	}

	return provider + "\x00" + account
}
