#!/bin/sh
set -eu

unit=${1:-deploy/systemd/pve-storage-guard-observer.service}

if ! command -v systemd-analyze >/dev/null 2>&1; then
    echo "systemd-analyze is required" >&2
    exit 1
fi

scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT HUP INT TERM
verification_unit="$scratch/pve-storage-guard-observer.service"

# Hosted CI intentionally has no PVE observer installed. Replace only the two
# executable paths in a disposable copy so systemd verifies the unit syntax;
# the Go contract test separately pins the production paths and arguments.
sed \
    -e 's|^ExecStartPre=.*|ExecStartPre=/bin/true|' \
    -e 's|^ExecStart=.*|ExecStart=/bin/true|' \
    "$unit" >"$verification_unit"

systemd-analyze verify "$verification_unit"
security_report=$(systemd-analyze security --offline=yes --no-pager "$unit")
printf '%s\n' "$security_report"
exposure=$(printf '%s\n' "$security_report" | sed -n 's/.*: \([0-9][0-9.]*\) .*/\1/p' | tail -n 1)
if [ -z "$exposure" ]; then
    echo "could not parse systemd security exposure" >&2
    exit 1
fi
awk -v exposure="$exposure" 'BEGIN { exit !(exposure <= 1.0) }' || {
    echo "systemd security exposure $exposure exceeds the 1.0 ceiling" >&2
    exit 1
}
