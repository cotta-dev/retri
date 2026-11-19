# Operations

Personal notes for maintaining and releasing retri.

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

Run "Build and Release" with **version left empty**. Download the development
`.deb` files from the run’s **Artifacts** section (retained for 3 days).
No tag or release is created.

## Local .deb Build (for completion testing)

Build a local amd64 `.deb` under `/tmp/retri-artifacts` without waiting for CI:

```bash
./scripts/build-local-deb.sh
```

## Install from .deb

```bash
sudo apt-get install -y --allow-downgrades /tmp/retri_VERSION_amd64.deb
```
