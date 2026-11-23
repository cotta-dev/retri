# Operations

Build, release, install, and update Retri. Run the commands below from your own
interactive terminal; publishing and installation change GitHub or the local system.

## Release

### GitHub Actions (recommended)

1. Open [Build and Release](https://github.com/cotta-dev/retri/actions/workflows/release.yml).
2. Click **Run workflow**, select **main**, and enter a new version such as `v1.2.3`.
3. Run the workflow. Vet, race-enabled tests, lint and both `.deb` builds must pass
   before the tag is pushed and the release is published with generated notes.

Manual production versions must be `vMAJOR.MINOR.PATCH`. The workflow uses the
commit selected at dispatch time, even if main changes during the build.
Existing releases and tags pointing to another commit are rejected; tags are never overwritten.
If publishing fails after pushing the tag but before creating the release, rerun
that original workflow run to retry with the same commit. Existing releases need
manual inspection before recovery.

### Local tag push (still supported)

Fetch and verify the intended release commit first. Replace `v1.2.3` with a new
version; this example tags the fetched `origin/main` commit:

```bash
git fetch origin main
git log -1 --oneline origin/main
git tag v1.2.3 origin/main
git push origin v1.2.3
```

Tag push triggers GoReleaser via GitHub Actions → publishes `.deb` (amd64/arm64) to GitHub Releases.

## Update

Run on the machine where Retri is installed. The updater downloads a newer
release's `.deb` for the running binary's architecture and invokes
`sudo apt-get install -y`. It requires Linux with APT, `sudo` permission, and
HTTPS access to GitHub. Published packages support `amd64` and `arm64`.
This installs a system package even if the current binary was built from source
or installed with `go install`; such a binary may still take precedence on `PATH`.

```bash
retri --update
```

Use `command -v retri` and `retri --version` to check which binary your shell
uses after installation.

Normal execution checks for a newer release at most once per local day and
only prints a notice. It never installs an update automatically. Disable that
check in the config with:

```yaml
defaults:
  update_check: false
```

This setting does not disable an explicit `retri --update`. Development builds
whose version is `dev` skip the normal daily check.

## Manual Build (for testing before release)

Run "Build and Release" with **version left empty**. Download the development
`.deb` files from the run’s **Artifacts** section (retained for 3 days).
No tag or release is created.

## Local .deb Build (for completion testing)

From the repository root, build a `.deb` for the local environment's Debian
architecture (`dpkg --print-architecture`) under `/tmp/retri-artifacts`.
Requires Bash, Git, a Go toolchain meeting [go.mod](../go.mod), `dpkg`, and
`dpkg-deb`. The script clears `/tmp/retri-local-deb` and replaces its target
package file before building:

```bash
./scripts/build-local-deb.sh
```

The default filename is `retri_0.0.0-local.COMMIT_ARCH.deb`. `VERSION` can
override the package version. `ARCH` changes package metadata only; it does
not select Go's compilation target. Use the release workflow for both supported
architectures.

## Install from .deb

Replace the version and architecture with the downloaded filename:

```bash
sudo apt-get install -y /tmp/retri_VERSION_amd64.deb
```

When deliberately installing an older development package over a newer release,
add `--allow-downgrades` explicitly.

The `.deb` automatically places bash, zsh, and fish completion files; no manual
generation or copying is needed. They load in new shells when the shell's
completion system is enabled. To load them immediately in the current Bash:

```bash
source /usr/share/bash-completion/completions/retri
```
