package provider

import (
	"context"

	"github.com/k0wl0n/gctx/pkg/config"
)

// fakeProvider is a scriptable Provider; unset function fields return zero values.
type fakeProvider struct {
	IDValue          ID
	LabelValue       string
	TargetLabelValue string
	CLIValue         CLIStatus

	ValidateTargetFn func(target string) error
	CreateFn         func(ctx context.Context, acct config.Account) (CreateOutcome, error)
	LoginFn          func(ctx context.Context, acct config.Account, lio LoginIO) (LoginResult, error)
	ActivateFn       func(ctx context.Context, acct config.Account) (ActivateResult, error)
	LocalFn          func(acct config.Account) LocalStatus
	CheckAuthFn      func(ctx context.Context, accts []config.Account) AuthReport
	EnvFn            func(acct config.Account) ([]string, error)
	DeleteFn         func(ctx context.Context, acct config.Account, opt DeleteOptions) (DeleteResult, error)
	DetailsFn        func(acct config.Account) []Field
}

var _ Provider = (*fakeProvider)(nil)

func (f *fakeProvider) ID() ID              { return f.IDValue }
func (f *fakeProvider) Label() string       { return f.LabelValue }
func (f *fakeProvider) TargetLabel() string { return f.TargetLabelValue }
func (f *fakeProvider) CLI() CLIStatus      { return f.CLIValue }

func (f *fakeProvider) ValidateTarget(target string) error {
	if f.ValidateTargetFn == nil {
		return nil
	}
	return f.ValidateTargetFn(target)
}

func (f *fakeProvider) Create(ctx context.Context, acct config.Account) (CreateOutcome, error) {
	if f.CreateFn == nil {
		return CreateOutcome{}, nil
	}
	return f.CreateFn(ctx, acct)
}

func (f *fakeProvider) Login(ctx context.Context, acct config.Account, lio LoginIO) (LoginResult, error) {
	if f.LoginFn == nil {
		return LoginResult{Account: acct.Name}, nil
	}
	return f.LoginFn(ctx, acct, lio)
}

func (f *fakeProvider) Activate(ctx context.Context, acct config.Account) (ActivateResult, error) {
	if f.ActivateFn == nil {
		return ActivateResult{Account: acct.Name}, nil
	}
	return f.ActivateFn(ctx, acct)
}

func (f *fakeProvider) Local(acct config.Account) LocalStatus {
	if f.LocalFn == nil {
		return LocalStatus{Status: HealthReady}
	}
	return f.LocalFn(acct)
}

func (f *fakeProvider) CheckAuth(ctx context.Context, accts []config.Account) AuthReport {
	if f.CheckAuthFn == nil {
		return AuthReport{}
	}
	return f.CheckAuthFn(ctx, accts)
}

func (f *fakeProvider) Env(acct config.Account) ([]string, error) {
	if f.EnvFn == nil {
		return nil, nil
	}
	return f.EnvFn(acct)
}

func (f *fakeProvider) Delete(ctx context.Context, acct config.Account, opt DeleteOptions) (DeleteResult, error) {
	if f.DeleteFn == nil {
		return DeleteResult{Account: acct.Name}, nil
	}
	return f.DeleteFn(ctx, acct, opt)
}

func (f *fakeProvider) Details(acct config.Account) []Field {
	if f.DetailsFn == nil {
		return nil
	}
	return f.DetailsFn(acct)
}
