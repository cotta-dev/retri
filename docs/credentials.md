# Credential providers

Retri can continue to use the existing `password`, `secret`, `${VAR}`,
`RETRI_SSH_PASSWORD`, and `RETRI_SSH_SECRET` configuration. Named credentials
are optional and do not require Bitwarden, Vaultwarden, or Linux keyrings.

Named credentials separate **where a secret comes from** from **where it is
used**:

```yaml
credentials:
  linux-login:
    provider: prompt
    prompt: "Linux SSH password"

  linux-sudo:
    provider: prompt
    prompt: "Linux sudo password"
    cache:
      backend: session-keyring
      ttl: 8h

defaults:
  password_credential: linux-login
  secret_credential: linux-sudo
```

A named prompt is requested once per Retri invocation even when many hosts use
it. With `session-keyring` caching, later Retri invocations in the same Linux
login session can reuse the value until its TTL expires.

## Providers

### `prompt`

Reads a hidden value from the terminal.

```yaml
credentials:
  common-sudo:
    provider: prompt
    prompt: "Common sudo password"
```

### `env`

Reads an environment variable. This has the same environment exposure tradeoffs
as Retri's existing environment-variable support.

```yaml
credentials:
  common-login:
    provider: env
    ref: COMMON_LOGIN_PASSWORD
```

### `keyring`

Reads an existing Linux session-keyring `user` key. This requires the `keyctl`
command from the Linux keyutils package.

```yaml
credentials:
  common-sudo:
    provider: keyring
    ref: retri-linux-sudo
```

### `bitwarden`

Reads a Bitwarden login item through the official `bw` CLI. The provider does
not distinguish Bitwarden Cloud from Vaultwarden; `bw config server` controls
the server endpoint. Retri does not log in to or unlock the vault for you.

```yaml
credentials:
  network-login:
    provider: bitwarden
    ref: 00000000-0000-0000-0000-000000000000
    field: password
```

`field` defaults to `password`. `username` and named custom fields are also
supported.

### `literal`

Stores the value directly in the Retri configuration. This exists for
completeness and migration but is not recommended for real secrets.

```yaml
credentials:
  test-only:
    provider: literal
    value: test-password
```

## Session-keyring cache

Caching is independent of the provider:

```yaml
credentials:
  network-login:
    provider: bitwarden
    ref: 00000000-0000-0000-0000-000000000000
    cache:
      backend: session-keyring
      ttl: 8h
```

Retri checks the cache before contacting the provider. This means a credential
previously obtained from Bitwarden/Vaultwarden can still be used during a
network outage while the key is present and unexpired.

`session-keyring` is Linux-only and requires `keyctl`. If `ttl` is omitted it
defaults to 8 hours. No persistent on-disk cache is created.

If an explicitly configured provider fails and no valid cache entry exists,
Retri returns an error instead of silently changing to another provider.
Credentials that are not configured at all retain Retri's existing hidden
fallback prompt behavior.

## Security boundaries

The session keyring reduces accidental exposure through configuration files,
process arguments, and inherited environment variables. It does **not** make a
plaintext password inaccessible to a fully privileged root attacker while
Retri is using that password. Where possible, prefer SSH public-key or FIDO2
authentication so no reusable SSH password needs to be handled at all.

Retri strips `RETRI_SSH_PASSWORD`, `RETRI_SSH_SECRET`, and `BW_SESSION` from the
environment of SSH and recorded-shell child processes. Provider processes that
need their own environment are invoked separately.
