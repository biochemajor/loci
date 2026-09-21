#!/usr/bin/env bash
# sign-windows.sh — Authenticode-sign loci's Windows self-extractor (.exe) so it
# stops triggering SmartScreen's "unknown publisher" warning.
#
# This uses osslsigncode, which runs on macOS/Linux, so you can sign the Windows
# .exe from the same Mac you build on:
#   macOS:  brew install osslsigncode
#   Linux:  apt-get install osslsigncode   (or build from source)
#
# On Windows itself you would instead use Microsoft's signtool:
#   signtool sign /fd SHA256 /tr http://timestamp.digicert.com /td SHA256 \
#     /f cert.pfx /p <password> archive-windows-amd64.exe
#
# You supply the certificate and password via environment variables; this script
# never contains or stores any secret.
#
# Required:
#   WIN_PFX        path to your code-signing certificate (.pfx / PKCS#12)
#   WIN_PFX_PASS   password for that .pfx
#
# Optional:
#   WIN_TS_URL     RFC3161 timestamp URL (default: http://timestamp.digicert.com)
#   WIN_NAME       program name embedded in the signature (default: loci)
#   WIN_URL        publisher URL embedded in the signature
#
# Usage:
#   ./build/sign-windows.sh dist/archive-windows-amd64.exe [more...]
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <windows-exe> [more...]" >&2
  exit 2
fi

command -v osslsigncode >/dev/null 2>&1 || {
  echo "error: 'osslsigncode' not found. Install it (brew install osslsigncode)," >&2
  echo "       or sign on Windows with signtool (see header of this script)." >&2
  exit 1
}

if [[ -z "${WIN_PFX:-}" || -z "${WIN_PFX_PASS:-}" ]]; then
  echo "error: set WIN_PFX (path to .pfx) and WIN_PFX_PASS (its password)." >&2
  exit 1
fi
[[ -f "$WIN_PFX" ]] || { echo "error: no such certificate file: $WIN_PFX" >&2; exit 1; }

ts_url="${WIN_TS_URL:-http://timestamp.digicert.com}"
name="${WIN_NAME:-loci}"

extra=()
[[ -n "${WIN_URL:-}" ]] && extra+=(-i "$WIN_URL")

for exe in "$@"; do
  [[ -f "$exe" ]] || { echo "error: no such file: $exe" >&2; exit 1; }
  echo ">> signing $exe"
  tmp="$exe.signed.$$"
  osslsigncode sign \
    -pkcs12 "$WIN_PFX" -pass "$WIN_PFX_PASS" \
    -n "$name" "${extra[@]}" \
    -t "$ts_url" -h sha256 \
    -in "$exe" -out "$tmp"
  mv -f "$tmp" "$exe"
  echo ">> verifying $exe"
  osslsigncode verify "$exe" | grep -Ei 'result|signer|timestamp' || true
done

echo "Done."
