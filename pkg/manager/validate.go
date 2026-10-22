package manager

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/k0wl0n/gctx/pkg/provider/gcp"
)

var (
	ErrInvalidName      = errors.New("invalid account name")
	ErrInvalidProjectID = gcp.ErrInvalidProjectID
	ErrAccountNotFound  = errors.New("account not found")
	ErrAccountExists    = errors.New("account already exists")
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
