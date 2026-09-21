#!/usr/bin/env bash
# sign-macos.sh — code-sign and notarize loci's macOS self-extractors so they
# open without a Gatekeeper prompt.
#
# You supply the identity and credentials via environment variables; this script
# never contains or stores any secret. Run it on a Mac with Xcode command line
# tools installed.
#
# Required:
#   SIGN_IDENTITY   Developer ID Application identity, exactly as shown by
#                   `security find-identity -v -p codesigning`, e.g.
#                   "Developer ID Application: Your Name (TEAMID123)"
#
# Notarization credentials — provide EITHER a stored keychain profile:
#   NOTARY_PROFILE  name from `xcrun notarytool store-credentials <profile>`
# OR the three Apple ID values:
#   APPLE_ID        your Apple ID email
#   APPLE_TEAM_ID   your 10-char Developer Team ID
#   APPLE_PASSWORD  an app-specific password (appleid.apple.com > Sign-In & Security)
#
# Optional:
#   SKIP_NOTARIZE=1 sign only, do not notarize (faster for local testing)
#
# Usage:
#   ./build/sign-macos.sh dist/archive-macos-arm64 [more binaries...]
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <macos-binary> [more...]" >&2
  exit 2
fi

need() { command -v "$1" >/dev/null 2>&1 || { echo "error: '$1' not found (install Xcode command line tools)" >&2; exit 1; }; }
need codesign
[[ -n "${SKIP_NOTARIZE:-}" ]] || need xcrun

if [[ -z "${SIGN_IDENTITY:-}" ]]; then
  echo "error: set SIGN_IDENTITY to your Developer ID Application identity." >&2
  echo "       list them with: security find-identity -v -p codesigning" >&2
  exit 1
fi

# Resolve how notarization will authenticate.
notary_args=()
if [[ -z "${SKIP_NOTARIZE:-}" ]]; then
  if [[ -n "${NOTARY_PROFILE:-}" ]]; then
    notary_args=(--keychain-profile "$NOTARY_PROFILE")
  elif [[ -n "${APPLE_ID:-}" && -n "${APPLE_TEAM_ID:-}" && -n "${APPLE_PASSWORD:-}" ]]; then
    notary_args=(--apple-id "$APPLE_ID" --team-id "$APPLE_TEAM_ID" --password "$APPLE_PASSWORD")
  else
    echo "error: set NOTARY_PROFILE, or APPLE_ID + APPLE_TEAM_ID + APPLE_PASSWORD," >&2
    echo "       or pass SKIP_NOTARIZE=1 to sign without notarizing." >&2
    exit 1
  fi
fi

for bin in "$@"; do
  [[ -f "$bin" ]] || { echo "error: no such file: $bin" >&2; exit 1; }
  echo ">> signing $bin"
  # --options runtime enables the hardened runtime, which notarization requires.
  codesign --force --timestamp --options runtime --sign "$SIGN_IDENTITY" "$bin"
  codesign --verify --strict --verbose=2 "$bin"

  if [[ -n "${SKIP_NOTARIZE:-}" ]]; then
    echo "   signed (notarization skipped)"
    continue
  fi

  echo ">> notarizing $bin"
  zip="$(mktemp -d)/$(basename "$bin").zip"
  # ditto produces the archive layout notarytool expects for a bare executable.
  ditto -c -k --keepParent "$bin" "$zip"
  xcrun notarytool submit "$zip" "${notary_args[@]}" --wait
  rm -f "$zip"

  # NOTE: a notarization ticket cannot be stapled onto a bare executable — only
  # onto a .app bundle, .dmg, or .pkg. The binary above IS notarized (Gatekeeper
  # verifies it via an online check on first run), but for fully offline,
  # prompt-free launches, wrap the binary in a .dmg or .pkg and staple THAT:
  #   xcrun stapler staple your.dmg
  echo "   notarized (see note in script about offline stapling)"
done

echo "Done. Verify assessment with: spctl -a -t exec -vv <binary>"
