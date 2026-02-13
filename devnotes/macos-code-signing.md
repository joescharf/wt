# macOS Code Signing and Notarization for wt

*2026-02-13T18:15:46Z*

This document describes the macOS code signing and notarization pipeline added to the `wt` project. Previously, the `wt` binary distributed via Homebrew was unsigned, causing macOS Gatekeeper security warnings for users. This change replicates the signing pipeline from the `pm` project using `pycodesign.py` as a GoReleaser post-hook.

## What Changed

Four files were created or modified:

- **`wt_pycodesign.ini`** (new) — Configuration for `pycodesign.py` specifying the Developer ID Application and Installer certificates, keychain profile, bundle ID, and paths.
- **`.goreleaser.yml`** (modified) — Added `env_files` for local release auth, a post-hook on `universal_binaries` to invoke `pycodesign.py`, and a `release` section that attaches the signed `.pkg` to GitHub releases.
- **`.github/workflows/release.yml`** (modified) — Converted from automatic tag-push trigger to manual `workflow_dispatch`, since code signing requires local macOS keychain access.
- **`Makefile`** (new) — Build, test, release, and docs targets. `make release` / `make release-local` run goreleaser with signing; `make release-snapshot` builds without publishing.

## Signing Pipeline

The pipeline is triggered as a GoReleaser post-hook on the `universal_binaries` step. After GoReleaser creates the universal (x86_64 + arm64) macOS binary, `pycodesign.py` runs and:

1. **Signs** the binary with the Developer ID Application certificate (hardened runtime enabled)
2. **Builds** a `.pkg` installer signed with the Developer ID Installer certificate
3. **Submits** the `.pkg` to Apple's notary service
4. **Staples** the notarization ticket to the `.pkg`

The signed `.pkg` is then attached to the GitHub release as an extra file.

## Verification

The snapshot build was run to verify the full signing pipeline without publishing:

```bash
codesign -dvv dist/wt_darwin_all/wt 2>&1 | head -8
```

```output
Executable=/Users/joescharf/app/wt/dist/wt_darwin_all/wt
Identifier=wt
Format=Mach-O universal (x86_64 arm64)
CodeDirectory v=20500 size=16590 flags=0x10000(runtime) hashes=513+2 location=embedded
Signature size=9048
Authority=Developer ID Application: Scharfnado LLC (PC9WL4QUXV)
Authority=Developer ID Certification Authority
Authority=Apple Root CA
```

```bash
pkgutil --check-signature dist/wt_macos_universal.pkg 2>&1 | head -6
```

```output
Package "wt_macos_universal.pkg":
   Status: signed by a developer certificate issued by Apple for distribution
   Notarization: trusted by the Apple notary service
   Signed with a trusted timestamp on: 2026-02-13 18:11:29 +0000
   Certificate Chain:
    1. Developer ID Installer: Scharfnado LLC (PC9WL4QUXV)
```

```bash
xcrun stapler validate dist/wt_macos_universal.pkg 2>&1
```

```output
Processing: /Users/joescharf/app/wt/dist/wt_macos_universal.pkg
The validate action worked!
```

## Release Workflow

To create a signed release locally:

1. Tag the release: `git tag v0.6.0 && git push origin v0.6.0`
2. Run: `make release` (or `make release-local`, identical)
3. GoReleaser builds, signs, notarizes, and creates a **draft** GitHub release
4. Review the draft release on GitHub, then publish

For unsigned CI fallback releases, use the GitHub Actions workflow dispatch with the tag name.

## Prerequisites

- `uv` — Python package runner (for pycodesign.py)
- `pycodesign.py` at `~/.local/bin/pycodesign.py`
- Apple Developer ID Application certificate in keychain
- Apple Developer ID Installer certificate in keychain
- Keychain profile `SCHARFNADO_LLC` (for notarytool)
- GoReleaser GitHub token at `~/.config/goreleaser/github_token`
- Homebrew tap token at `~/.config/goreleaser/homebrew_tap_token` (for pushing formula to `joescharf/homebrew-tap`)
