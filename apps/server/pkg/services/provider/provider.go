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
