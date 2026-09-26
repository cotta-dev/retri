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

Reads an existing Linux session-keyring `user` key using the
kernel API directly; Retri does not require the `keyctl` command.

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
    server: https://vault.bitwarden.com  # must match bw status serverUrl
    account: "BITWARDEN-USER-ID"          # bw status userId (not email)
    ref: 00000000-0000-0000-0000-000000000000
    field: password
```

`field` defaults to `password`. `username` and named custom fields are also
supported.

### Vaultwarden setup

Retri uses the official `bw` CLI. Install it on the same WSL environment where
you run Retri, then point it at the externally reachable Vaultwarden URL:

```bash
bw config server https://vault.example.com
bw login your-email@example.com
bw unlock
```

Run the `export BW_SESSION=...` command printed by `bw unlock` in the same shell.
Check the active profile without printing vault items:

```bash
bw status
```

Copy `serverUrl` and `userId` from that output. `account` must use `userId`, not
the email address. Create a Login item in Vaultwarden and obtain its item ID,
for example by searching item names:

```bash
bw list items --search retri
```

Then reference the item from Retri. The `field` value is `password` by default;
it can also be `username` or the name of a custom field.

```yaml
credentials:
  vaultwarden-ssh:
    provider: bitwarden
    server: https://vault.example.com
    account: "00000000-0000-0000-0000-000000000000"
    ref: "VAULTWARDEN-ITEM-ID"
    field: password

defaults:
  password_credential: vaultwarden-ssh
```

Use a second named credential for a separate sudo or enable secret. If the
Vaultwarden item changed in another client, refresh the local CLI vault before
running Retri:

```bash
bw sync
```

`bw` is not needed by users of other providers. Retri does not perform login,
unlock, lock, logout, or sync automatically.

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
    server: https://vault.bitwarden.com  # must match bw status serverUrl
    account: "BITWARDEN-USER-ID"          # bw status userId (not email)
    ref: 00000000-0000-0000-0000-000000000000
    cache:
      backend: session-keyring
      ttl: 8h
```

Retri checks the cache before contacting the provider. This means a credential
previously obtained from Bitwarden/Vaultwarden can still be used during a
network outage while the key is present and unexpired.

`session-keyring` is Linux-only and uses the kernel API directly. If `ttl` is omitted it
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

Retri strips `RETRI_SSH_PASSWORD`, `RETRI_SSH_SECRET`, `BW_*`, `BWS_ACCESS_TOKEN`,
all configured env-provider variables, and variables referenced by legacy or
literal credential expansions from SSH and recorded-shell child environments.
The `bw` child retains its own Bitwarden environment, including `BW_SESSION`.
Ordinary SSH agent, proxy and locale variables are preserved. This is not an
allowlist of every possible application secret in the parent environment.

## Resolution and lifecycle

Configuration priority is `defaults < groups < device_types < hosts < CLI`.
The last applicable group wins. `RETRI_SSH_PASSWORD` / `RETRI_SSH_SECRET` are
fallbacks only when no config value is selected (including an empty legacy
`${VAR}` expansion). Literal and named references are exclusive within one
layer; either form can override the other at a higher layer. CLI `-p/-s` values
are literal, not expanded, and may be visible in process arguments.

Named providers must return a nonempty value without NUL, CR or LF. Their
failure never triggers a legacy env or prompt fallback. An explicit prompt
requires terminal stdin. Completely unset credentials remain optional without
a terminal, allowing public-key authentication and non-sudo automation.
`${VAR}` expansion applies to legacy password/secret and literal `value`, not
provider `ref`, `server`, `account`, `field`, or prompt labels.

Resolved values and errors are shared by name for one invocation. Distinct
prompts are serialized. Host execution uses that invocation's snapshot even if
the session cache expires during the run; TTL is a limit on reuse by a later
invocation, not a deadline that interrupts an established SSH session.

Session cache TTL defaults to 8h and accepts 1s through 24h. A shortened TTL is
also enforced against the original acquisition time. Reads do not extend it.
Cache identity includes the canonical config path, credential name, source,
reference, field, and bound Bitwarden profile. A changed literal definition is
checked inside the protected payload rather than exposed in public metadata.
Changing an env variable or rotating an upstream secret does not automatically
refresh an unexpired snapshot.

```bash
retri -c config.yaml --credential-cache-refresh -g production
retri -c config.yaml --credential-cache-clear
```

Refresh deletes each selected cached credential before contacting its source;
failure does not restore the old value. Clear removes current definitions and
exits without contacting providers. Removed/renamed definitions expire at their
old TTL. Cache access or storage errors fail explicitly; they are not misses.
Concurrent invocations may obtain different snapshots; no cross-process prompt
deduplication is promised. Each stored payload carries its own expiry.

The Linux session keyring is inherited by child processes and its scope depends
on PAM/service setup; it is not necessarily one terminal or one human login.
Cache keys grant permissions to their owner only, but this does not isolate
secrets from other processes running as that user. Each snapshot gets a dedicated
keyring with owner-only permissions and a kernel timeout before any secret is
published inside it. The value key also receives permissions before publication
and its own timeout afterwards (updating a user key resets its timeout). The
containing ring bounds retention even if publication is interrupted. Failed
publication revokes the keys. No plaintext disk
cache is written. Go strings/runtime copies, swap, crash dumps and privileged
memory inspection are outside this protection; temporary byte buffers are
cleared where practical, without claiming guaranteed memory erasure.

The v2 cache ignores entries written by earlier versions of this unmerged PR.
If you ran those versions, inspect and remove their `retri:credential:` keys
with keyutils; an earlier failed timeout could have left an unbounded entry.

## Bitwarden operational contract

`bw` is required only when a Bitwarden source is actually contacted. Retri uses
`--nointeraction`, a 30-second command timeout, and bounded stdout; provider
stderr and response bodies are never included in errors. Retri does not manage
login, unlock, lock, logout, or sync. Export `BW_SESSION` from your own unlocked
CLI session, and run `bw sync` when you need changes made by another client.
`get item` reads the CLI vault; expiry of Retri's cache does not guarantee that
this vault has been synchronized with the server.

For Bitwarden session caching, `server` and `account` are required. Use the exact
`serverUrl` and `userId` from `bw status` (not the email address). On a miss, Retri
checks those values and unlocked status before reading the item. A cache hit
needs neither `bw` nor an unlocked vault or network. Lock/logout does not revoke
Retri's independent copy: clear it explicitly. Switching accounts or servers
requires updating the bound profile, which selects a different cache identity.

Automated logs redact known credential values across read boundaries and again
after terminal rendering; debug output also redacts literal values. This does
not promise detection of arbitrary secret transformations by a remote peer.
Interactive record mode cannot redact credentials Retri has never acquired.
