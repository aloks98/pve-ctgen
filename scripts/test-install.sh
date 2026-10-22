#!/usr/bin/env bash
# Offline installer smoke test: verify checksum failures preserve the installed file.
set -euo pipefail
repo_dir="$(cd "$(dirname "$0")/.." && pwd)"
fixture_dir="$(mktemp -d)"
trap 'rm -rf "$fixture_dir"' EXIT
mkdir -p "$fixture_dir/bin" "$fixture_dir/install" "$fixture_dir/archive"
printf '#!/bin/sh\necho "pvectgen 1.2.3"\n' > "$fixture_dir/archive/pvectgen"
chmod +x "$fixture_dir/archive/pvectgen"
archive_name="pvectgen_1.2.3_$(uname -s | tr '[:upper:]' '[:lower:]')_amd64.tar.gz"
case "$(uname -m)" in aarch64|arm64) archive_name="${archive_name%amd64.tar.gz}arm64.tar.gz" ;; esac
tar -czf "$fixture_dir/$archive_name" -C "$fixture_dir/archive" pvectgen
(cd "$fixture_dir" && if command -v sha256sum >/dev/null 2>&1; then sha256sum "$archive_name"; else shasum -a 256 "$archive_name"; fi > checksums.txt)
cat > "$fixture_dir/bin/curl" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
out=""
while [[ $# -gt 0 ]]; do
 case "$1" in -o) out="$2"; shift 2 ;; *) url="$1"; shift ;; esac
done
cp "$PVECTGEN_FIXTURE/${url##*/}" "$out"
MOCK
chmod +x "$fixture_dir/bin/curl"
export PATH="$fixture_dir/bin:$PATH"
export PVECTGEN_FIXTURE="$fixture_dir"
bash "$repo_dir/scripts/install.sh" --dir "$fixture_dir/install" v1.2.3 > "$fixture_dir/log"
[[ "$("$fixture_dir/install/pvectgen" version)" == 'pvectgen 1.2.3' ]]
printf 'keep-existing' > "$fixture_dir/install/pvectgen"
printf '%064d  %s\n' 0 "$archive_name" > "$fixture_dir/checksums.txt"
if bash "$repo_dir/scripts/install.sh" --dir "$fixture_dir/install" v1.2.3 > "$fixture_dir/log" 2>&1; then
 echo 'Installer accepted an invalid checksum'; exit 1
fi
[[ "$(cat "$fixture_dir/install/pvectgen")" == 'keep-existing' ]]
echo 'Installer verification and atomic replacement tests passed'
