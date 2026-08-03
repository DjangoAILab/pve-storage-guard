#!/usr/bin/env python3
"""Rehearse observer start, restart, cold start, and exact rollback in CI."""

from __future__ import annotations

import argparse
import grp
import hashlib
import json
import os
from pathlib import Path
import pwd
import shutil
import signal
import stat
import subprocess
import sys
import time


GUARD_NAME = "PVE_STORAGE_GUARD_SYSTEMD_REHEARSAL"
GUARD_VALUE = "ephemeral-ci-v1"
SERVICE_USER = "pve-storage-guard"
SERVICE_GROUP = "pve-storage-guard"
UNIT_NAME = "pve-storage-guard-observer.service"
UNIT_TARGET = Path("/run/systemd/system") / UNIT_NAME
BINARY_TARGET = Path("/usr/local/bin/pve-storage-guard")
CONFIG_DIRECTORY = Path("/etc/pve-storage-guard")
CONFIG_TARGET = CONFIG_DIRECTORY / "agent.json"
PVE_DIRECTORY = Path("/etc/pve")
PVE_SHIM = Path("/usr/bin/pvesh")
ZPOOL_SHIM = Path("/usr/sbin/zpool")
FIXTURE_TARGET = Path("/usr/local/share/pve-storage-guard-rehearsal")
BACKUP_DIRECTORY = Path("/run/pve-storage-guard-rehearsal-backup")
SYSLOG_IDENTIFIER = "pve-storage-guard-observer"
SCHEMA_VERSION = "guard.storage-slo.io/v1alpha1"
REQUIRED_FIXTURES = (
    "cluster-status.json",
    "storage-config.json",
    "storage-status.json",
    "zpool-iostat-w.txt",
    "zpool-iostat-wpH.txt",
)
MUTATION_TARGETS = (
    UNIT_TARGET,
    BINARY_TARGET,
    CONFIG_DIRECTORY,
    PVE_DIRECTORY,
    PVE_SHIM,
    ZPOOL_SHIM,
    FIXTURE_TARGET,
    BACKUP_DIRECTORY,
)


class RehearsalError(Exception):
    """A categorical lifecycle rehearsal failure."""


def _lexists(path: Path) -> bool:
    return os.path.lexists(path)


def _sha256_bytes(payload: bytes) -> str:
    return hashlib.sha256(payload).hexdigest()


def _read_regular(path: Path, *, executable: bool = False, max_bytes: int = 64 * 1024 * 1024) -> bytes:
    try:
        before = os.lstat(path)
    except OSError:
        raise RehearsalError("trusted input is unavailable") from None
    mode = stat.S_IMODE(before.st_mode)
    if (
        stat.S_ISLNK(before.st_mode)
        or not stat.S_ISREG(before.st_mode)
        or mode & 0o022
        or before.st_size > max_bytes
        or (executable and not mode & 0o111)
    ):
        raise RehearsalError("trusted input metadata is unsafe")
    flags = os.O_RDONLY | getattr(os, "O_CLOEXEC", 0) | getattr(os, "O_NOFOLLOW", 0)
    try:
        descriptor = os.open(path, flags)
    except OSError:
        raise RehearsalError("trusted input is unsafe") from None
    try:
        opened = os.fstat(descriptor)
        chunks = []
        while True:
            chunk = os.read(descriptor, 1024 * 1024)
            if not chunk:
                break
            chunks.append(chunk)
            if sum(len(value) for value in chunks) > max_bytes:
                raise RehearsalError("trusted input exceeds size limit")
        final = os.fstat(descriptor)
    finally:
        os.close(descriptor)
    if not os.path.samestat(before, opened) or not os.path.samestat(opened, final):
        raise RehearsalError("trusted input changed while reading")
    return b"".join(chunks)


def _command(arguments, *, check=True, timeout=30, accepted=(0,)):
    environment = {
        "HOME": "/nonexistent",
        "PATH": "/usr/sbin:/usr/bin:/sbin:/bin",
        "LC_ALL": "C",
        "LANG": "C",
        "TZ": "UTC",
    }
    try:
        result = subprocess.run(
            list(arguments),
            stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            env=environment,
            text=True,
            timeout=timeout,
            check=False,
        )
    except (OSError, subprocess.TimeoutExpired):
        raise RehearsalError("lifecycle command could not complete") from None
    if check and result.returncode not in accepted:
        raise RehearsalError("lifecycle command failed")
    return result


def _identity_absent() -> bool:
    try:
        pwd.getpwnam(SERVICE_USER)
    except KeyError:
        pass
    else:
        return False
    try:
        grp.getgrnam(SERVICE_GROUP)
    except KeyError:
        return True
    return False


def _trusted_root_directory(path: Path) -> bool:
    try:
        metadata = os.lstat(path)
    except OSError:
        return False
    return (
        stat.S_ISDIR(metadata.st_mode)
        and not stat.S_ISLNK(metadata.st_mode)
        and metadata.st_uid == 0
        and metadata.st_gid == 0
        and not stat.S_IMODE(metadata.st_mode) & 0o022
    )


def _validate_source_directory(path: Path) -> dict[str, bytes]:
    try:
        metadata = os.lstat(path)
    except OSError:
        raise RehearsalError("fixture directory is unavailable") from None
    if stat.S_ISLNK(metadata.st_mode) or not stat.S_ISDIR(metadata.st_mode):
        raise RehearsalError("fixture directory is unsafe")
    return {name: _read_regular(path / name, max_bytes=1024 * 1024) for name in REQUIRED_FIXTURES}


def _binary_version(path: Path) -> str:
    result = _command((str(path), "version"), timeout=10)
    value = result.stdout.strip()
    if (
        result.stderr
        or len(result.stdout.splitlines()) != 1
        or not value
        or len(value) > 64
        or any(not (character.isalnum() or character in "._+-") for character in value)
    ):
        raise RehearsalError("binary version is invalid")
    return value


def _preflight(baseline: Path, candidate: Path, unit: Path, fixtures: Path):
    if os.environ.get(GUARD_NAME) != GUARD_VALUE:
        raise RehearsalError("explicit ephemeral-CI guard is required")
    if os.environ.get("CI") != "true":
        raise RehearsalError("CI environment is required")
    if sys.platform != "linux" or os.geteuid() != 0:
        raise RehearsalError("root Linux execution is required")
    try:
        init_name = Path("/proc/1/comm").read_text(encoding="ascii").strip()
    except OSError:
        raise RehearsalError("PID 1 identity is unavailable") from None
    if init_name != "systemd":
        raise RehearsalError("systemd must be PID 1")
    if any(_lexists(path) for path in MUTATION_TARGETS):
        raise RehearsalError("rehearsal target already exists")
    parent_directories = {
        UNIT_TARGET.parent,
        BINARY_TARGET.parent,
        CONFIG_DIRECTORY.parent,
        PVE_SHIM.parent,
        ZPOOL_SHIM.parent,
        FIXTURE_TARGET.parent,
        BACKUP_DIRECTORY.parent,
    }
    unsafe_parents = sorted(str(path) for path in parent_directories if not _trusted_root_directory(path))
    if unsafe_parents:
        # Every candidate is a fixed public system path defined above; no
        # caller-provided or host-identity value is included in this error.
        raise RehearsalError("rehearsal parent directory is unsafe: " + ",".join(unsafe_parents))
    if not _identity_absent():
        raise RehearsalError("rehearsal service identity already exists")
    fixed_path = "/usr/sbin:/usr/bin:/sbin:/bin"
    for executable in ("systemctl", "journalctl", "useradd", "userdel", "groupadd", "groupdel"):
        if shutil.which(executable, path=fixed_path) is None:
            raise RehearsalError("required lifecycle command is unavailable")

    baseline_payload = _read_regular(baseline, executable=True)
    candidate_payload = _read_regular(candidate, executable=True)
    unit_payload = _read_regular(unit, max_bytes=64 * 1024)
    fixture_payloads = _validate_source_directory(fixtures)
    if baseline_payload == candidate_payload:
        raise RehearsalError("candidate binary is not byte-distinct")
    baseline_version = _binary_version(baseline)
    candidate_version = _binary_version(candidate)
    if baseline_version == candidate_version:
        raise RehearsalError("candidate version is not distinct")
    if b"User=pve-storage-guard" not in unit_payload or b"ExecStart=/usr/local/bin/pve-storage-guard " not in unit_payload:
        raise RehearsalError("unit is not the production observer contract")
    return {
        "baseline": baseline_payload,
        "candidate": candidate_payload,
        "unit": unit_payload,
        "fixtures": fixture_payloads,
        "baseline_version": baseline_version,
        "candidate_version": candidate_version,
    }


def _fsync_directory(path: Path):
    descriptor = os.open(path, os.O_RDONLY | getattr(os, "O_DIRECTORY", 0))
    try:
        os.fsync(descriptor)
    finally:
        os.close(descriptor)


def _atomic_write(path: Path, payload: bytes, mode: int, uid: int, gid: int):
    temporary = path.with_name("." + path.name + ".rehearsal-next")
    if _lexists(temporary):
        raise RehearsalError("temporary target already exists")
    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, "O_CLOEXEC", 0) | getattr(os, "O_NOFOLLOW", 0)
    descriptor = os.open(temporary, flags, mode)
    try:
        os.fchown(descriptor, uid, gid)
        os.fchmod(descriptor, mode)
        view = memoryview(payload)
        while view:
            written = os.write(descriptor, view)
            if written <= 0:
                raise RehearsalError("artifact write was incomplete")
            view = view[written:]
        os.fsync(descriptor)
    except Exception:
        try:
            temporary.unlink()
        except OSError:
            pass
        raise
    finally:
        os.close(descriptor)
    os.replace(temporary, path)
    _fsync_directory(path.parent)


def _choose_kernel_device() -> str:
    try:
        lines = Path("/proc/diskstats").read_text(encoding="ascii").splitlines()
    except OSError:
        raise RehearsalError("diskstats is unavailable") from None
    for line in lines:
        fields = line.split()
        if len(fields) >= 14 and fields[2] not in {"", ".", ".."}:
            return fields[2]
    raise RehearsalError("diskstats has no usable device")


def _config_payload(domain: str, kernel_device: str) -> bytes:
    document = {
        "apiVersion": SCHEMA_VERSION,
        "kind": "PVEAgentConfig",
        "spec": {
            "domainKey": domain,
            "node": "fixture-node",
            "storage": "fixture-storage",
            "zpool": "fixturepool",
            "sampleIntervalSeconds": 1,
            "commandTimeoutSeconds": 5,
            "emergencyWaitMilliseconds": 100,
            "resources": [
                {
                    "resourceKey": "fixture-resource",
                    "kernelDevice": kernel_device,
                    "root": False,
                    "critical": False,
                }
            ],
        },
    }
    return (json.dumps(document, sort_keys=True, separators=(",", ":")) + "\n").encode("utf-8")


def _pvesh_shim() -> bytes:
    script = """#!/bin/sh
set -eu
case "$*" in
  'get /cluster/status --output-format json') file=cluster-status.json ;;
  'get /storage/fixture-storage --output-format json') file=storage-config.json ;;
  'get /nodes/fixture-node/storage/fixture-storage/status --output-format json') file=storage-status.json ;;
  *) exit 64 ;;
esac
exec /usr/bin/cat /usr/local/share/pve-storage-guard-rehearsal/$file
"""
    return script.encode("ascii")


def _zpool_shim() -> bytes:
    script = """#!/bin/sh
set -eu
case "$*" in
  'iostat -w fixturepool') file=zpool-iostat-w.txt ;;
  'iostat -wpH -y fixturepool 1 1') file=zpool-iostat-wpH.txt ;;
  *) exit 64 ;;
esac
exec /usr/bin/cat /usr/local/share/pve-storage-guard-rehearsal/$file
"""
    return script.encode("ascii")


def _service_property(name: str) -> str:
    result = _command(("systemctl", "show", UNIT_NAME, "--property", name, "--value"))
    return result.stdout.strip()


def _wait_active(previous_pid: int | None = None, timeout: float = 50) -> int:
    deadline = time.monotonic() + timeout
    expected_uid = pwd.getpwnam(SERVICE_USER).pw_uid
    while time.monotonic() < deadline:
        if _service_property("ActiveState") == "active":
            value = _service_property("MainPID")
            if value.isdigit() and int(value) > 1 and int(value) != previous_pid:
                pid = int(value)
                try:
                    status_text = Path(f"/proc/{pid}/status").read_text(encoding="ascii")
                except OSError:
                    time.sleep(0.2)
                    continue
                uid_line = next((line for line in status_text.splitlines() if line.startswith("Uid:")), "")
                fields = uid_line.split()
                if len(fields) == 5 and all(field == str(expected_uid) for field in fields[1:]):
                    return pid
        time.sleep(0.2)
    raise RehearsalError("observer did not become active as non-root")


def _observations() -> list[dict]:
    result = _command(
        (
            "journalctl",
            "--unit",
            UNIT_NAME,
            "--identifier",
            SYSLOG_IDENTIFIER,
            "--output",
            "cat",
            "--no-pager",
        )
    )
    observations = []
    for line in result.stdout.splitlines():
        if not line.strip():
            continue
        try:
            value = json.loads(line)
        except json.JSONDecodeError:
            raise RehearsalError("observer journal contains non-JSON output") from None
        if not isinstance(value, dict):
            raise RehearsalError("observer journal record is invalid")
        observations.append(value)
    return observations


def _validate_observation(value: dict, domain: str):
    required = {
        "schemaVersion",
        "id",
        "observedAt",
        "domainKey",
        "writeWaitP95Milliseconds",
        "waitValid",
        "emergency",
        "managementPlaneHealthy",
    }
    optional = {"waitEvidence", "ioPressure", "diskSignals"}
    if not required.issubset(value) or not set(value).issubset(required | optional):
        raise RehearsalError("observer journal record shape is invalid")
    if (
        value["schemaVersion"] != SCHEMA_VERSION
        or value["domainKey"] != domain
        or not isinstance(value["id"], str)
        or not value["id"].startswith("observation-")
        or not isinstance(value["managementPlaneHealthy"], bool)
        or value["managementPlaneHealthy"] is not True
    ):
        raise RehearsalError("observer journal record binding is invalid")
    signals = value.get("diskSignals")
    if not isinstance(signals, list) or len(signals) != 1 or signals[0].get("resourceKey") != "fixture-resource":
        raise RehearsalError("observer journal disk binding is invalid")


def _new_watch_identifier(value: dict, domain: str, known_ids: set[str]) -> str | None:
    identifier = value.get("id")
    # ExecStartPre emits a PVEInventory record under the same journal
    # identifier. It is valid service output, but not a watch sample.
    if not isinstance(identifier, str) or value.get("domainKey") != domain or identifier in known_ids:
        return None
    _validate_observation(value, domain)
    return identifier


def _wait_new_observations(domain: str, known_ids: set[str], minimum: int = 1, timeout: float = 20) -> set[str]:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        found = set()
        for value in _observations():
            identifier = _new_watch_identifier(value, domain, known_ids)
            if identifier is not None:
                found.add(identifier)
        if len(found) >= minimum:
            return found
        time.sleep(0.25)
    raise RehearsalError("observer did not emit required lifecycle samples")


def _all_observation_ids() -> set[str]:
    return {value.get("id") for value in _observations() if isinstance(value.get("id"), str)}


def _assert_stopped():
    _command(("systemctl", "stop", UNIT_NAME), timeout=20)
    if _service_property("ActiveState") != "inactive" or _service_property("MainPID") not in {"", "0"}:
        raise RehearsalError("observer did not reach inactive state")
    control_group = _service_property("ControlGroup")
    if control_group:
        processes = Path("/sys/fs/cgroup") / control_group.lstrip("/") / "cgroup.procs"
        if processes.exists() and processes.read_text(encoding="ascii").strip():
            raise RehearsalError("observer cgroup retained processes after stop")


def _install_runtime(payloads):
    _command(("groupadd", "--system", SERVICE_GROUP))
    _command(
        (
            "useradd",
            "--system",
            "--gid",
            SERVICE_GROUP,
            "--home-dir",
            "/nonexistent",
            "--shell",
            "/usr/sbin/nologin",
            "--no-create-home",
            SERVICE_USER,
        )
    )
    account = pwd.getpwnam(SERVICE_USER)
    group = grp.getgrnam(SERVICE_GROUP)
    if account.pw_uid == 0 or account.pw_gid != group.gr_gid:
        raise RehearsalError("service identity is unsafe")

    PVE_DIRECTORY.mkdir(mode=0o700)
    os.chmod(PVE_DIRECTORY, 0o755)
    CONFIG_DIRECTORY.mkdir(mode=0o700)
    os.chown(CONFIG_DIRECTORY, 0, group.gr_gid)
    os.chmod(CONFIG_DIRECTORY, 0o750)
    FIXTURE_TARGET.mkdir(mode=0o700)
    BACKUP_DIRECTORY.mkdir(mode=0o700)
    for name, content in payloads["fixtures"].items():
        _atomic_write(FIXTURE_TARGET / name, content, 0o444, 0, 0)
    os.chmod(FIXTURE_TARGET, 0o555)
    _atomic_write(PVE_SHIM, _pvesh_shim(), 0o555, 0, 0)
    _atomic_write(ZPOOL_SHIM, _zpool_shim(), 0o555, 0, 0)
    _atomic_write(UNIT_TARGET, payloads["unit"], 0o644, 0, 0)
    return account.pw_uid, group.gr_gid


def _remove_bounded_directory(path: Path, allowed_names: set[str]):
    try:
        metadata = os.lstat(path)
    except OSError:
        return
    if stat.S_ISLNK(metadata.st_mode) or not stat.S_ISDIR(metadata.st_mode):
        return
    try:
        entries = list(os.scandir(path))
    except OSError:
        return
    if any(entry.name not in allowed_names or entry.is_dir(follow_symlinks=False) for entry in entries):
        return
    for entry in entries:
        try:
            os.unlink(entry.path)
        except OSError:
            return
    try:
        path.rmdir()
    except OSError:
        pass


def _cleanup():
    try:
        _command(("systemctl", "stop", UNIT_NAME), check=False, timeout=20)
    except RehearsalError:
        pass
    for path in (UNIT_TARGET, BINARY_TARGET, CONFIG_TARGET, PVE_SHIM, ZPOOL_SHIM):
        try:
            if path.is_symlink() or path.is_file():
                path.unlink()
        except OSError:
            pass
    _remove_bounded_directory(
        FIXTURE_TARGET,
        set(REQUIRED_FIXTURES) | {"." + name + ".rehearsal-next" for name in REQUIRED_FIXTURES},
    )
    _remove_bounded_directory(
        BACKUP_DIRECTORY,
        {"pve-storage-guard", "agent.json", ".pve-storage-guard.rehearsal-next", ".agent.json.rehearsal-next"},
    )
    _remove_bounded_directory(CONFIG_DIRECTORY, {".agent.json.rehearsal-next"})
    try:
        PVE_DIRECTORY.rmdir()
    except OSError:
        pass
    try:
        _command(("systemctl", "daemon-reload"), check=False)
    except RehearsalError:
        pass
    try:
        _command(("userdel", SERVICE_USER), check=False)
    except RehearsalError:
        pass
    try:
        _command(("groupdel", SERVICE_GROUP), check=False)
    except RehearsalError:
        pass


def _verify_cleanup():
    if any(_lexists(path) for path in MUTATION_TARGETS) or not _identity_absent():
        raise RehearsalError("rehearsal cleanup was incomplete")
    load_state = _command(("systemctl", "show", UNIT_NAME, "--property", "LoadState", "--value"), check=False)
    if load_state.returncode == 0 and load_state.stdout.strip() not in {"", "not-found"}:
        raise RehearsalError("systemd retained the rehearsal unit")


def rehearse(baseline: Path, candidate: Path, unit: Path, fixtures: Path) -> dict:
    payloads = _preflight(baseline, candidate, unit, fixtures)
    os.umask(0o077)
    completed = False
    result = None
    try:
        service_uid, service_gid = _install_runtime(payloads)
        kernel_device = _choose_kernel_device()
        baseline_config = _config_payload("rehearsal-baseline", kernel_device)
        candidate_config = _config_payload("rehearsal-candidate", kernel_device)
        baseline_binary_digest = _sha256_bytes(payloads["baseline"])
        candidate_binary_digest = _sha256_bytes(payloads["candidate"])
        baseline_config_digest = _sha256_bytes(baseline_config)
        candidate_config_digest = _sha256_bytes(candidate_config)
        if baseline_config_digest == candidate_config_digest:
            raise RehearsalError("candidate config is not byte-distinct")

        _atomic_write(BINARY_TARGET, payloads["baseline"], 0o755, 0, 0)
        _atomic_write(CONFIG_TARGET, baseline_config, 0o600, service_uid, service_gid)
        _atomic_write(BACKUP_DIRECTORY / "pve-storage-guard", payloads["baseline"], 0o500, 0, 0)
        _atomic_write(BACKUP_DIRECTORY / "agent.json", baseline_config, 0o400, 0, 0)
        _command(("systemctl", "daemon-reload"))
        enabled = _command(("systemctl", "is-enabled", UNIT_NAME), check=False)
        if enabled.returncode == 0:
            raise RehearsalError("rehearsal unit must not be enabled")

        known = _all_observation_ids()
        _command(("systemctl", "start", UNIT_NAME), timeout=30)
        initial_pid = _wait_active()
        initial_ids = _wait_new_observations("rehearsal-baseline", known, minimum=2, timeout=25)

        known |= initial_ids
        os.kill(initial_pid, signal.SIGKILL)
        restarted_pid = _wait_active(previous_pid=initial_pid, timeout=50)
        restart_ids = _wait_new_observations("rehearsal-baseline", known, timeout=20)

        _assert_stopped()
        known |= restart_ids
        _command(("systemctl", "reset-failed", UNIT_NAME))
        _atomic_write(BINARY_TARGET, payloads["candidate"], 0o755, 0, 0)
        _atomic_write(CONFIG_TARGET, candidate_config, 0o600, service_uid, service_gid)
        if _sha256_bytes(_read_regular(BINARY_TARGET, executable=True)) != candidate_binary_digest:
            raise RehearsalError("candidate binary installation failed")
        _command(("systemctl", "start", UNIT_NAME), timeout=30)
        candidate_pid = _wait_active()
        candidate_ids = _wait_new_observations("rehearsal-candidate", known, timeout=20)
        if candidate_pid in {initial_pid, restarted_pid}:
            raise RehearsalError("candidate cold start reused an old PID")

        _assert_stopped()
        known |= candidate_ids
        _command(("systemctl", "reset-failed", UNIT_NAME))
        baseline_backup = _read_regular(BACKUP_DIRECTORY / "pve-storage-guard", executable=True)
        config_backup = _read_regular(BACKUP_DIRECTORY / "agent.json")
        _atomic_write(BINARY_TARGET, baseline_backup, 0o755, 0, 0)
        _atomic_write(CONFIG_TARGET, config_backup, 0o600, service_uid, service_gid)
        if (
            _sha256_bytes(_read_regular(BINARY_TARGET, executable=True)) != baseline_binary_digest
            or _sha256_bytes(_read_regular(CONFIG_TARGET)) != baseline_config_digest
        ):
            raise RehearsalError("rollback digest restoration failed")
        _command(("systemctl", "start", UNIT_NAME), timeout=30)
        rollback_pid = _wait_active()
        rollback_ids = _wait_new_observations("rehearsal-baseline", known, timeout=20)
        if rollback_pid in {initial_pid, restarted_pid, candidate_pid}:
            raise RehearsalError("rollback cold start reused an old PID")
        _assert_stopped()
        enabled = _command(("systemctl", "is-enabled", UNIT_NAME), check=False)
        if enabled.returncode == 0:
            raise RehearsalError("rehearsal unit became enabled")

        result = {
            "schemaVersion": "guard.storage-slo.io/systemd-lifecycle-rehearsal/v1alpha1",
            "kind": "SystemdLifecycleRehearsal",
            "platform": "ephemeral-ubuntu-ci",
            "syntheticFixtures": True,
            "policyEvidenceEligible": False,
            "serviceNonRoot": True,
            "initialSamples": len(initial_ids),
            "supervisedRestartSamples": len(restart_ids),
            "candidateColdStartSamples": len(candidate_ids),
            "rollbackColdStartSamples": len(rollback_ids),
            "initialPid": initial_pid,
            "restartedPid": restarted_pid,
            "candidatePid": candidate_pid,
            "rollbackPid": rollback_pid,
            "baselineBinarySha256": "sha256:" + baseline_binary_digest,
            "candidateBinarySha256": "sha256:" + candidate_binary_digest,
            "baselineConfigSha256": "sha256:" + baseline_config_digest,
            "candidateConfigSha256": "sha256:" + candidate_config_digest,
            "rollbackBinaryExact": True,
            "rollbackConfigExact": True,
            "unitEnabled": False,
            "requestedProductionMutations": 0,
        }
        completed = True
    finally:
        _cleanup()
        _verify_cleanup()
    if not completed or result is None:
        raise RehearsalError("lifecycle rehearsal did not complete")
    result["cleanupComplete"] = True
    return result


def _parser():
    parser = argparse.ArgumentParser(description="Rehearse the observer systemd lifecycle on an ephemeral CI VM.")
    parser.add_argument("--baseline-binary", required=True)
    parser.add_argument("--candidate-binary", required=True)
    parser.add_argument("--unit", required=True)
    parser.add_argument("--fixtures", required=True)
    return parser


def main(argv=None) -> int:
    arguments = _parser().parse_args(argv)
    try:
        result = rehearse(
            Path(arguments.baseline_binary),
            Path(arguments.candidate_binary),
            Path(arguments.unit),
            Path(arguments.fixtures),
        )
    except RehearsalError as error:
        print("systemd lifecycle rehearsal: {}".format(error), file=sys.stderr)
        return 1
    print(json.dumps(result, sort_keys=True, separators=(",", ":")))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
