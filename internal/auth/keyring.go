// Package auth manages GitHub personal access tokens stored in the OS keychain.
//
// Precedence resolved by Token():
//  1. GITHUB_TOKEN env var (escape hatch — never persisted)
//  2. Keychain entry for the active account
//
// Multi-account: the account ID is resolved upstream (--account flag,
// GHUB_ACCOUNT env, or the saved "default" account) and passed in.
package auth

import (
	"errors"
	"os"

	"github.com/zalando/go-keyring"
)

// Service is the keyring service name. Stable across versions.
const Service = "ghub"

// DefaultAccountKey is the keyring user whose value is the active account ID.
// Stored separately from per-account PATs so `ghub auth use <id>` is a single
// write.
const DefaultAccountKey = "__default__"

// ErrNotFound is returned when no token is configured for the given account.
var ErrNotFound = errors.New("no token configured")

// Token returns the GitHub token for the active account.
// Honors the GITHUB_TOKEN env override first.
func Token(account string) (string, error) {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t, nil
	}
	if account == "" {
		a, err := DefaultAccount()
		if err != nil {
			return "", err
		}
		account = a
	}
	tok, err := keyring.Get(Service, account)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", ErrNotFound
		}
		return "", err
	}
	return tok, nil
}

// Set stores a token for the given account ID.
func Set(account, token string) error {
	return keyring.Set(Service, account, token)
}

// Delete removes the token for the given account ID.
func Delete(account string) error {
	err := keyring.Delete(Service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

// DefaultAccount returns the saved active-account ID, or ErrNotFound if unset.
func DefaultAccount() (string, error) {
	v, err := keyring.Get(Service, DefaultAccountKey)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", ErrNotFound
		}
		return "", err
	}
	return v, nil
}

// SetDefaultAccount records which account ID is active.
func SetDefaultAccount(account string) error {
	return keyring.Set(Service, DefaultAccountKey, account)
}
