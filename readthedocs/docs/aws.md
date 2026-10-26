# AWS

Prerequisite: the `aws` CLI v2 on `PATH`. A missing CLI marks only AWS accounts `CLI MISSING`.

An AWS account is an AWS profile (`gctx create prod prod-profile --provider aws`). The profile can be adopted or created:

- **Adopt** (default, `auth=profile`): the profile must already exist in `~/.aws/config` (or `$AWS_CONFIG_FILE`), otherwise `create` fails with a clear error.
- **SSO** (`auth=sso`): the provider writes the profile with `aws configure set` from `sso_start_url`, `sso_region` and `region`, plus `sso_account_id` and `sso_role_name` if you supply them. Otherwise add those two once (`aws configure set profile.<name>.sso_account_id <id>`, `... .sso_role_name <role>`); credential checks fail without them, and `gctx login` prints the exact commands when they are missing. gctx never writes tokens or secrets, and never edits your AWS config on delete.

AWS has no global switch, so `gctx switch` prints a hint instead of changing state. Select the profile per shell:

```bash
eval "$(gctx env prod)"   # exports AWS_PROFILE=prod-profile
gctx login prod           # aws sso login --profile=prod-profile (SSO profiles only)
```

Credential checks use `aws sts get-caller-identity --profile=<profile>`. Profile names must start with a letter, digit or underscore, so a name can never be read as a flag.

The delete dialog offers `aws sso logout` (off by default). It signs out of every AWS SSO session on this machine, not only this profile: other AWS accounts need `gctx login` again afterwards.


`gctx login` prints any warnings (for example missing `sso_account_id` or `sso_role_name`) in its output. The TUI create form (`c`) offers the same two modes and shows what will be written to `~/.aws/config` before it does.
