# AWS

Prerequisite: the `aws` CLI v2 on `PATH`. A missing CLI marks only AWS accounts `CLI MISSING`.

An AWS account is an AWS profile (`gctx create prod prod-profile --provider aws`). The profile can be adopted or created:

- **Adopt** (default, `auth=profile`): the profile must already exist in `~/.aws/config` (or `$AWS_CONFIG_FILE`), otherwise `create` fails with a clear error.
- **SSO** (`auth=sso`): the provider writes the profile with `aws configure set` from `sso_start_url`, `sso_region` and `region`. The account id and role are set after login. gctx never writes tokens or secrets, and never edits your AWS config on delete.

AWS has no global switch, so `gctx switch` prints a hint instead of changing state. Select the profile per shell:

```bash
eval "$(gctx env prod)"   # exports AWS_PROFILE=prod-profile
gctx login prod           # aws sso login --profile prod-profile (SSO profiles only)
```

Credential checks use `aws sts get-caller-identity --profile <profile>`.

