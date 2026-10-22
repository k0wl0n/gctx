package manager

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/k0wl0n/gctx/pkg/config"
	"github.com/k0wl0n/gctx/pkg/provider"
	"github.com/k0wl0n/gctx/pkg/provider/gcp"
)

var (
	ErrInvalidName      = errors.New("invalid account name")
	ErrInvalidProjectID = gcp.ErrInvalidProjectID
	ErrAccountNotFound  = errors.New("account not found")
	ErrAccountExists    = errors.New("account already exists")
	ErrUnknownProvider  = errors.New("unknown provider")
)

var accountNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// ValidateAccountName checks the gcloud configuration-name rule applied to
// "<name>-config". It also keeps the name safe to use in file paths.
func ValidateAccountName(name string) error {
	if !accountNamePattern.MatchString(name) {
		return fmt.Errorf("%w %q: names must start with a lowercase letter and contain only lowercase letters, digits, and hyphens", ErrInvalidName, name)
	}
	return nil
}

// ValidateProjectID checks a GCP project ID, including the domain-scoped
// "<domain>:<id>" form.
func ValidateProjectID(projectID string) error { return gcp.ValidateProjectID(projectID) }

type notFoundError struct{ name string }

func (e notFoundError) Error() string        { return fmt.Sprintf("account '%s' not found", e.name) }
func (e notFoundError) Is(target error) bool { return target == ErrAccountNotFound }

type existsError struct{ name string }

func (e existsError) Error() string        { return fmt.Sprintf("account '%s' already exists", e.name) }
func (e existsError) Is(target error) bool { return target == ErrAccountExists }

// unknownProviderError is an account whose provider this build does not
// know, typically written by a newer version.
type unknownProviderError struct {
	account string
	id      provider.ID
}

func (e unknownProviderError) Error() string {
	return fmt.Sprintf("account '%s' uses provider %q, which this version of %s does not support", e.account, e.id, config.AppName)
}
func (e unknownProviderError) Is(target error) bool { return target == ErrUnknownProvider }
