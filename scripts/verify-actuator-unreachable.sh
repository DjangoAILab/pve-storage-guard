#!/bin/sh
set -eu

module=github.com/DjangoAILab/pve-storage-guard
actuator_package="$module/internal/actuator/pve"

if go list -deps ./cmd/pve-storage-guard | grep -Fqx "$actuator_package"; then
    echo "pve-storage-guard unexpectedly depends on the PVE actuator package" >&2
    exit 1
fi

scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT HUP INT TERM
binary="$scratch/pve-storage-guard"
symbols="$scratch/symbols.txt"

# Disable inlining so a future accidental backend link cannot hide the
# constructor or method symbols from this independent binary-level check.
go build -buildvcs=false -gcflags=all=-l -o "$binary" ./cmd/pve-storage-guard
go tool nm "$binary" >"$symbols"

if grep -Eq 'internal/actuator/pve.*(localBackend|newLocalBackend|osPveshExecutor)' "$symbols"; then
    echo "pve-storage-guard unexpectedly contains the dormant local backend" >&2
    exit 1
fi

echo "actuator reachability gate passed: CLI dependency and binary are actuator-free"
