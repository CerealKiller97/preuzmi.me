// Package payments builds the lookup key that identifies a receipt's paid state.
//
// The paid state itself is stored in the payments table of the receipts database
// (see the receipts repository), the single source of truth. This package holds
// only the key derivation so the HTTP layer, the CLI and the database all agree
// on how a (period, provider) pair maps to a key.
package payments

import "strings"

// Key builds the identifier for a receipt's paid state. The period is expected
// to already be normalized by the caller, so that "010-2025" and "10-2025"
// cannot produce two different entries for the same receipt.
func Key(period, provider string) string {
	return strings.ToLower(period + "|" + provider)
}
