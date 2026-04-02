# Operations

Personal notes for maintaining and releasing retri.

## Release

```bash
git tag v0.1.0
git push origin v0.1.0
```

Tag push triggers GoReleaser via GitHub Actions → publishes `.deb` (amd64/arm64) to GitHub Releases.

## Update (on server)

```bash
retri --update
```

## Manual Build (for testing before release)

Trigger "Build and Release" workflow manually from GitHub Actions UI.
Downloads the dev `.deb` from the `dev` pre-release on the Releases page.

## Install from .deb

```bash
sudo apt-get install -y --allow-downgrades /tmp/retri_VERSION_amd64.deb
```
