# loci

A small macOS app (plus a scriptable CLI) that packs a set of files into a
**self-extracting archive** for Windows and macOS. It:

- computes a **SHA-256 checksum of every file before compression**,
- zips the files and records the **checksum of the archive after compression**,
- optionally **encrypts** the archive with a password (AES-256-GCM),
- produces a **self-extracting executable** per target OS that, when run,
  extracts the files, **renames** each with a configurable **prefix and/or
  suffix**, and **verifies** every file against its pre-compression checksum.

## Is a single dual-OS self-extractor possible?

No — and that part of the original idea is worth being precise about. Executable
formats are OS-specific: Windows uses PE (`.exe`), macOS uses Mach-O. A single
file cannot be a trustworthy native executable for both. So loci does the
standard, robust thing: from **one build** it emits **one self-extractor per
target** — a Windows `.exe` and a macOS binary (Apple Silicon and/or Intel) —
each carrying the same encrypted payload. Everything else you asked for is fully
supported.

Each self-extractor is a native Go binary with the archive appended after it:

```
[ extractor stub (native PE / Mach-O) ][ container: header + payload ][ trailer ]
```

The stub reads its own file at run time, finds the payload via the trailer,
decrypts if needed, unzips, renames, and verifies checksums. No runtime (no
Python/Java) is required on the recipient's machine.

## Build

Requires Go 1.24+. On a Mac you also need the Xcode command line tools
(`xcode-select --install`) to compile the GUI.

```sh
# 1. Generate the embedded extractor stubs (Windows/macOS/Linux). Pure Go,
#    cross-compiles from any host.
./build/build-stubs.sh          # or: make stubs

# 2a. Run the desktop GUI builder (macOS):
make gui                        # go run ./cmd/loci-gui

# 2b. ...or build the command-line builder:
make cli                        # -> dist/loci-cli

# Package a double-clickable macOS .app (optional):
go install fyne.io/fyne/v2/cmd/fyne@latest
make app                        # -> loci.app
```

## GUI usage

1. **Add File… / Add Folder…** to choose what to pack (folders are added
   recursively, preserving their structure).
2. Set an optional **Prefix** and/or **Suffix** (suffix is inserted before the
   file extension, e.g. `report.pdf` → `SECURE_report_v2.pdf`).
3. Optionally tick **Encrypt with a password** and enter it twice.
4. Choose **target platforms** (Windows / macOS Apple Silicon / macOS Intel).
5. Pick an **output folder** and base name, then **Build**.

Outputs are named `<base>-<target>[.exe]`, e.g. `archive-windows-amd64.exe`,
`archive-macos-arm64`.

## CLI usage

```sh
# Encrypted, renamed, for Windows + Apple Silicon:
dist/loci-cli -o release -prefix "SECURE_" -encrypt \
  -targets windows-amd64,macos-arm64 ./docs report.pdf

# Unencrypted, all default targets:
dist/loci-cli -o out ./photos

dist/loci-cli -list-targets      # show platforms and whether stubs are built
```

Flags: `-o` output base, `-prefix`, `-suffix`, `-encrypt` (prompts for a
password), `-password` (non-interactive; less safe), `-targets` (comma list).

## Extracting (what the recipient does)

- **Windows:** double-click the `.exe`. A console window opens, prompts for the
  password if the archive is encrypted, and extracts next to the `.exe` into a
  `<name>_extracted` folder. SmartScreen may warn about an unknown publisher —
  choose *More info → Run anyway*.
- **macOS:** in Terminal, `chmod +x <file>` then `./<file>`, or right-click →
  Open the first time. Because the binary is unsigned, Gatekeeper blocks a plain
  double-click until you approve it (or run
  `xattr -d com.apple.quarantine <file>`).

The extractor prints a per-file checksum verification result (e.g.
`Checksum verified: 3/3`) and fails loudly if any file doesn't match.

## Security notes

- Encryption is **AES-256-GCM**; the key is derived from your password with
  **PBKDF2-HMAC-SHA256** (200k iterations) over a random salt. GCM
  authentication means a wrong password or any tampering is rejected, not
  silently mis-extracted.
- With **no password**, the archive is stored unencrypted (compression only) —
  anyone with the file can extract it. Use a password if the contents are
  sensitive.
- loci produces **unsigned** binaries. To distribute without OS warnings, sign
  them after building — see below.

## Signing for distribution

Unsigned self-extractors work but warn: macOS Gatekeeper blocks the first launch
and Windows SmartScreen flags an "unknown publisher". Signing removes both. loci
ships scripts that sign the outputs in place; you provide the certificates and
credentials via environment variables (the scripts never store secrets).

You must obtain the signing identities yourself — an **Apple Developer Program**
membership (~$99/yr) for macOS, and a **code-signing certificate** from a CA
(~$100–400/yr) for Windows.

**macOS** — `build/sign-macos.sh` runs `codesign` (hardened runtime) then
notarizes with `notarytool`:

```sh
export SIGN_IDENTITY="Developer ID Application: Your Name (TEAMID123)"
export NOTARY_PROFILE="loci-notary"   # from: xcrun notarytool store-credentials
./build/sign-macos.sh dist/archive-macos-arm64 dist/archive-macos-amd64
```

A notarization ticket can't be stapled onto a bare executable — the binary is
notarized (Gatekeeper checks it online on first run), but for fully offline,
prompt-free launches wrap it in a `.dmg`/`.pkg` and staple that.

**Windows** — `build/sign-windows.sh` uses `osslsigncode`, so you can
Authenticode-sign the `.exe` from your Mac (`brew install osslsigncode`):

```sh
export WIN_PFX="/path/to/cert.pfx"
export WIN_PFX_PASS="…"
./build/sign-windows.sh dist/archive-windows-amd64.exe
```

On Windows you can instead use Microsoft's `signtool` (see the script header).
Convenience targets: `make sign-macos BINS="…"` and `make sign-windows BINS="…"`.

## Layout

```
internal/sfx/         core format, pack, extract, checksums, crypto (tested)
internal/stubassets/  embeds the prebuilt extractor stubs
stub/                 the extractor embedded in every output
cmd/loci-cli/         command-line builder
cmd/loci-gui/         Fyne desktop builder (macOS)
build/build-stubs.sh  cross-compiles the stubs into the embed directory
```

Run the tests with `make test` (`go test ./internal/...`).
