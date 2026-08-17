# Installation

Three paths. The first one does not need Go installed, and it is the only one
that matters to someone who just wants to run a test.

## 1. Download the release binary

On [releases](https://github.com/Diegobraun/braunrate/releases), take the file
for your system, check the checksum and unpack it:

```bash
# Linux/macOS. Replace VERSION, SYSTEM (linux, darwin) and ARCHITECTURE (amd64, arm64).
curl -fsSLO https://github.com/Diegobraun/braunrate/releases/download/vVERSION/braunrate_VERSION_SYSTEM_ARCHITECTURE.tar.gz
curl -fsSLO https://github.com/Diegobraun/braunrate/releases/download/vVERSION/checksums.txt
shasum -a 256 -c checksums.txt --ignore-missing
tar xzf braunrate_VERSION_SYSTEM_ARCHITECTURE.tar.gz
./braunrate version
```

On Windows, download the `.zip`, unpack it and run `braunrate.exe version`.

The archive carries the binary, the `README.md`, the `LICENSE` and the whole
`examples/` folder: downloading the tool and having no scenario to run leaves you
exactly where you were.

### First run on macOS

The system blocks a downloaded binary that was not notarized, with a message
saying the developer could not be verified. Clear it once:

```bash
xattr -d com.apple.quarantine ./braunrate
```

### First run on Windows

SmartScreen warns that the publisher is unknown. Click **More info** and then
**Run anyway**.

> **Note** Both warnings have the same cause: there is no code signature. The
> reason is in [What is left out](#what-is-left-out-and-why).

## 2. `go install`

For anyone who already has Go:

```bash
go install github.com/Diegobraun/braunrate/cmd/braunrate@latest
```

The binary comes out with no version injected: `braunrate version` answers `dev`,
and the result document keeps `dev`. That is deliberate: it is not the release
artefact, and a result document carrying a version number would be naming a
build it did not come from.

## 3. Build from source

For anyone going to touch the code:

```bash
git clone https://github.com/Diegobraun/braunrate
cd braunrate
go build -o braunrate ./cmd/braunrate
```

## Platforms

The release is born as a draft, which does not show up on the releases page and
has no download URL. It only becomes public after the binary is downloaded from
that draft, checked against the checksum and executed. A failed check discards
the draft, and there is no window in which someone downloads a broken artefact.

Six targets are published. Three of them are downloaded and executed before the
promotion; the other three come out compiled and packaged by the same
configuration, with nobody running them:

| Platform | Published | Executed at publication |
|---|---|---|
| linux/amd64 | yes | yes |
| linux/arm64 | yes | **no** |
| darwin/arm64 (Apple Silicon) | yes | yes |
| darwin/amd64 (Intel Mac) | yes | **no** |
| windows/amd64 | yes | yes |
| windows/arm64 | yes | **no** |

The check is not "the binary exists": it runs `version` and requires it to match
the tag, validates a published scenario and executes another one, requiring the
version to show up inside the result document.

## What is left out, and why

- **Code signing and notarization** (macOS and Windows). They require a paid
  certificate and a developer account in someone's name. Until those exist, both
  first-run warnings keep appearing, and the instruction to get past them stays
  written down instead of hidden.
- **Homebrew, Scoop and winget.** Each one is a repository to maintain and a
  release cadence to keep. A tap that falls behind is worse than one that does
  not exist, because it installs an old version quietly.
- **A graphical installer, `.msi`, `.pkg`, `.deb`.** The binary installs nothing,
  writes to no registry and needs no service: an installer would only add a step
  between downloading and running.
- **Self-update.** A measurement tool that updates itself changes the version
  between two runs somebody was about to compare. Comparing runs of different
  versions already comes out with no verdict; updating quietly would make that
  "no verdict" appear for no visible reason.
