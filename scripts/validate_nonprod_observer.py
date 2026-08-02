#!/usr/bin/env python3
"""Generate identity-free evidence for a read-only non-production PVE run."""

import argparse
import hashlib
import json
import math
import os
from pathlib import Path
import re
import selectors
import signal
import stat
import subprocess
import sys
import time


SCHEMA_VERSION = "guard.storage-slo.io/v1alpha1"
VALIDATOR_VERSION = "v1alpha1"
MAX_FILE_BYTES = 64 * 1024
MAX_OUTPUT_BYTES = 1024 * 1024
OPAQUE_KEY = re.compile(r"^[a-z][a-z0-9-]{0,62}$")
PRIVATE_ID = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$")
DIGEST = re.compile(r"^sha256:[0-9a-f]{64}$")
VERSION = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$")


class ValidationError(Exception):
    """A deliberately categorical validation failure."""


class DuplicateKeyError(ValueError):
    pass


def _strict_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise DuplicateKeyError()
        result[key] = value
    return result


def _reject_constant(_value):
    raise ValueError()


def _decode_json(payload, phase):
    try:
        value = json.loads(
            payload.decode("utf-8"),
            object_pairs_hook=_strict_object,
            parse_constant=_reject_constant,
        )
    except (UnicodeDecodeError, json.JSONDecodeError, DuplicateKeyError, ValueError):
        raise ValidationError("{} JSON is invalid".format(phase)) from None
    if not isinstance(value, dict):
        raise ValidationError("{} structure is invalid".format(phase))
    return value


def _exact_keys(value, required, optional=()):
    if not isinstance(value, dict):
        return False
    keys = set(value)
    return set(required).issubset(keys) and keys.issubset(set(required) | set(optional))


def _number(value, minimum=None, maximum=None, exclusive_minimum=False):
    if isinstance(value, bool) or not isinstance(value, (int, float)) or not math.isfinite(value):
        return False
    if minimum is not None:
        if exclusive_minimum and value <= minimum:
            return False
        if not exclusive_minimum and value < minimum:
            return False
    if maximum is not None and value > maximum:
        return False
    return True


def _read_private_config(path, effective_uid):
    if not path.is_absolute():
        raise ValidationError("config path must be absolute")
    try:
        before = os.lstat(path)
    except OSError:
        raise ValidationError("config file is unavailable") from None
    if stat.S_ISLNK(before.st_mode) or not stat.S_ISREG(before.st_mode) or before.st_size > MAX_FILE_BYTES:
        raise ValidationError("config file is unsafe")
    if before.st_uid != effective_uid:
        raise ValidationError("config ownership is unsafe")
    if stat.S_IMODE(before.st_mode) & 0o077:
        raise ValidationError("config permissions are unsafe")

    flags = os.O_RDONLY
    flags |= getattr(os, "O_CLOEXEC", 0)
    flags |= getattr(os, "O_NOFOLLOW", 0)
    try:
        descriptor = os.open(path, flags)
    except OSError:
        raise ValidationError("config file is unsafe") from None
    try:
        after = os.fstat(descriptor)
        if not os.path.samestat(before, after) or after.st_size > MAX_FILE_BYTES:
            raise ValidationError("config changed while reading")
        chunks = []
        remaining = MAX_FILE_BYTES + 1
        while remaining:
            chunk = os.read(descriptor, min(65536, remaining))
            if not chunk:
                break
            chunks.append(chunk)
            remaining -= len(chunk)
        payload = b"".join(chunks)
        if len(payload) > MAX_FILE_BYTES:
            raise ValidationError("config file is unsafe")
        final = os.fstat(descriptor)
        if not os.path.samestat(before, final) or final.st_size != len(payload):
            raise ValidationError("config changed while reading")
    finally:
        os.close(descriptor)

    document = _decode_json(payload, "config")
    if not _exact_keys(document, ("apiVersion", "kind", "spec")):
        raise ValidationError("config structure is invalid")
    if document["apiVersion"] != SCHEMA_VERSION or document["kind"] != "PVEAgentConfig":
        raise ValidationError("config structure is invalid")
    spec = document["spec"]
    spec_keys = (
        "domainKey",
        "node",
        "storage",
        "zpool",
        "sampleIntervalSeconds",
        "commandTimeoutSeconds",
        "emergencyWaitMilliseconds",
        "resources",
    )
    if not _exact_keys(spec, spec_keys):
        raise ValidationError("config structure is invalid")
    if not isinstance(spec["domainKey"], str) or not OPAQUE_KEY.fullmatch(spec["domainKey"]):
        raise ValidationError("config structure is invalid")
    for key in ("node", "storage", "zpool"):
        value = spec[key]
        if (
            not isinstance(value, str)
            or not PRIVATE_ID.fullmatch(value)
            or value in (".", "..")
            or ".." in value
        ):
            raise ValidationError("config structure is invalid")
    if (
        isinstance(spec["sampleIntervalSeconds"], bool)
        or not isinstance(spec["sampleIntervalSeconds"], int)
        or not 1 <= spec["sampleIntervalSeconds"] <= 60
        or isinstance(spec["commandTimeoutSeconds"], bool)
        or not isinstance(spec["commandTimeoutSeconds"], int)
        or not spec["sampleIntervalSeconds"] < spec["commandTimeoutSeconds"] <= 120
        or not _number(spec["emergencyWaitMilliseconds"], 0, exclusive_minimum=True)
    ):
        raise ValidationError("config structure is invalid")
    resources = spec["resources"]
    if not isinstance(resources, list) or not 1 <= len(resources) <= 64:
        raise ValidationError("config structure is invalid")
    resource_keys = set()
    devices = set()
    for resource in resources:
        if not _exact_keys(resource, ("resourceKey", "kernelDevice", "root", "critical")):
            raise ValidationError("config structure is invalid")
        key = resource["resourceKey"]
        device = resource["kernelDevice"]
        if (
            not isinstance(key, str)
            or not OPAQUE_KEY.fullmatch(key)
            or not isinstance(device, str)
            or not PRIVATE_ID.fullmatch(device)
            or ".." in device
            or not isinstance(resource["root"], bool)
            or not isinstance(resource["critical"], bool)
            or key in resource_keys
            or device in devices
        ):
            raise ValidationError("config structure is invalid")
        resource_keys.add(key)
        devices.add(device)
    private_values = {spec["node"], spec["storage"], spec["zpool"]} | devices
    return {
        "domain": spec["domainKey"],
        "resources": resource_keys,
        "private": private_values,
        "command_timeout": spec["commandTimeoutSeconds"],
    }


def _file_digest(path):
    digest = hashlib.sha256()
    flags = os.O_RDONLY | getattr(os, "O_CLOEXEC", 0) | getattr(os, "O_NOFOLLOW", 0)
    try:
        descriptor = os.open(path, flags)
    except OSError:
        raise ValidationError("binary file is unsafe") from None
    try:
        opened = os.fstat(descriptor)
        while True:
            chunk = os.read(descriptor, 1024 * 1024)
            if not chunk:
                break
            digest.update(chunk)
    finally:
        os.close(descriptor)
    return "sha256:" + digest.hexdigest(), opened


def _binary_identity(path, effective_uid, expected_digest):
    if not path.is_absolute():
        raise ValidationError("binary path must be absolute")
    if not DIGEST.fullmatch(expected_digest):
        raise ValidationError("expected binary digest is invalid")
    try:
        before = os.lstat(path)
    except OSError:
        raise ValidationError("binary file is unavailable") from None
    if stat.S_ISLNK(before.st_mode) or not stat.S_ISREG(before.st_mode):
        raise ValidationError("binary file is unsafe")
    if before.st_uid not in (0, effective_uid):
        raise ValidationError("binary ownership is unsafe")
    if stat.S_IMODE(before.st_mode) & 0o022 or not stat.S_IMODE(before.st_mode) & 0o111:
        raise ValidationError("binary permissions are unsafe")
    actual_digest, opened = _file_digest(path)
    if not os.path.samestat(before, opened):
        raise ValidationError("binary changed during validation")
    if actual_digest != expected_digest:
        raise ValidationError("binary digest mismatch")
    return (
        before.st_dev,
        before.st_ino,
        before.st_size,
        before.st_mtime_ns,
        before.st_mode,
        before.st_uid,
        actual_digest,
    )


def _assert_binary_unchanged(path, identity, phase, launch_hook):
    if launch_hook is not None:
        launch_hook(phase, path)
    try:
        current = os.lstat(path)
    except OSError:
        raise ValidationError("binary changed during validation") from None
    current_prefix = (
        current.st_dev,
        current.st_ino,
        current.st_size,
        current.st_mtime_ns,
        current.st_mode,
        current.st_uid,
    )
    if current_prefix != identity[:6]:
        raise ValidationError("binary changed during validation")
    digest, opened = _file_digest(path)
    if not os.path.samestat(current, opened) or digest != identity[6]:
        raise ValidationError("binary changed during validation")


def _fixed_environment():
    return {
        "HOME": "/nonexistent",
        "PATH": "/usr/sbin:/usr/bin:/sbin:/bin",
        "LC_ALL": "C",
        "LANG": "C",
        "TZ": "UTC",
    }


def _kill_group(process):
    if process.poll() is None:
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except (ProcessLookupError, PermissionError):
            try:
                process.kill()
            except ProcessLookupError:
                pass
    try:
        process.wait(timeout=2)
    except subprocess.TimeoutExpired:
        pass


def _close_pipes(process):
    for stream in (process.stdout, process.stderr):
        if stream is not None:
            stream.close()


def _spawn(binary, arguments, phase, identity, launch_hook):
    _assert_binary_unchanged(binary, identity, phase, launch_hook)
    try:
        return subprocess.Popen(
            [str(binary)] + list(arguments),
            stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            env=_fixed_environment(),
            close_fds=True,
            start_new_session=True,
        )
    except OSError:
        raise ValidationError("{} failed".format(phase)) from None


def _bounded_pipes(process, phase, timeout):
    selector = selectors.DefaultSelector()
    selector.register(process.stdout, selectors.EVENT_READ, "stdout")
    selector.register(process.stderr, selectors.EVENT_READ, "stderr")
    output = {"stdout": bytearray(), "stderr": bytearray()}
    deadline = time.monotonic() + timeout
    try:
        while selector.get_map():
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                _kill_group(process)
                raise ValidationError("{} timed out".format(phase))
            events = selector.select(min(remaining, 0.1))
            if not events and process.poll() is not None:
                events = [(key, selectors.EVENT_READ) for key in selector.get_map().values()]
            for key, _mask in events:
                try:
                    chunk = os.read(key.fileobj.fileno(), 65536)
                except BlockingIOError:
                    continue
                if not chunk:
                    selector.unregister(key.fileobj)
                    continue
                target = output[key.data]
                target.extend(chunk)
                if len(target) > MAX_OUTPUT_BYTES:
                    _kill_group(process)
                    raise ValidationError("{} output exceeded safety limit".format(phase))
        remaining = max(0.0, deadline - time.monotonic())
        try:
            return_code = process.wait(timeout=remaining)
        except subprocess.TimeoutExpired:
            _kill_group(process)
            raise ValidationError("{} timed out".format(phase)) from None
    finally:
        selector.close()
        _close_pipes(process)
    if output["stderr"].strip():
        raise ValidationError("{} wrote stderr".format(phase))
    if return_code != 0:
        raise ValidationError("{} failed".format(phase))
    return bytes(output["stdout"])


def _run_one_shot(binary, arguments, phase, identity, launch_hook, timeout):
    process = _spawn(binary, arguments, phase, identity, launch_hook)
    return _bounded_pipes(process, phase, timeout)


def _walk_strings(value):
    if isinstance(value, dict):
        for key, child in value.items():
            yield key
            yield from _walk_strings(child)
    elif isinstance(value, list):
        for child in value:
            yield from _walk_strings(child)
    elif isinstance(value, str):
        yield value


def _assert_private_absent(value, private_values, phase):
    for candidate in _walk_strings(value):
        for private in private_values:
            if candidate == private or (len(private) >= 3 and private in candidate):
                raise ValidationError("private identity appeared in {}".format(phase))


def _validate_inventory(value, config):
    if not _exact_keys(
        value,
        ("schemaVersion", "kind", "observedAt", "domainKey", "storageType", "resources"),
    ):
        raise ValidationError("inventory structure is invalid")
    if (
        value["schemaVersion"] != SCHEMA_VERSION
        or value["kind"] != "PVEInventory"
        or value["domainKey"] != config["domain"]
        or value["storageType"] != "zfspool"
        or not isinstance(value["observedAt"], str)
        or not isinstance(value["resources"], list)
        or not 1 <= len(value["resources"]) <= 64
    ):
        raise ValidationError("inventory structure is invalid")
    resource_keys = set()
    for resource in value["resources"]:
        if not _exact_keys(resource, ("resourceKey", "root", "critical")):
            raise ValidationError("inventory structure is invalid")
        if (
            resource["resourceKey"] not in config["resources"]
            or not isinstance(resource["root"], bool)
            or not isinstance(resource["critical"], bool)
        ):
            raise ValidationError("inventory structure is invalid")
        resource_keys.add(resource["resourceKey"])
    if resource_keys != config["resources"]:
        raise ValidationError("inventory structure is invalid")


def _validate_wait_evidence(value):
    keys = (
        "measurementLayer",
        "statistic",
        "source",
        "provenance",
        "sampleIntervalSeconds",
        "sampleWeight",
        "bucketUpperBoundNanoseconds",
    )
    return (
        _exact_keys(value, keys)
        and value["measurementLayer"] == "storage-domain"
        and value["statistic"] == "p95-upper-bound"
        and value["source"] == "openzfs-total-wait-histogram"
        and value["provenance"] == "observed"
        and isinstance(value["sampleIntervalSeconds"], int)
        and not isinstance(value["sampleIntervalSeconds"], bool)
        and 1 <= value["sampleIntervalSeconds"] <= 60
        and _number(value["sampleWeight"], 0, exclusive_minimum=True)
        and isinstance(value["bucketUpperBoundNanoseconds"], int)
        and not isinstance(value["bucketUpperBoundNanoseconds"], bool)
        and value["bucketUpperBoundNanoseconds"] >= 1
    )


def _validate_observation(value, config, phase):
    required = (
        "schemaVersion",
        "id",
        "observedAt",
        "domainKey",
        "writeWaitP95Milliseconds",
        "waitValid",
        "emergency",
        "managementPlaneHealthy",
    )
    optional = ("waitEvidence", "ioPressure", "diskSignals")
    if not _exact_keys(value, required, optional):
        raise ValidationError("{} structure is invalid".format(phase))
    if (
        value["schemaVersion"] != SCHEMA_VERSION
        or not isinstance(value["id"], str)
        or not 1 <= len(value["id"]) <= 256
        or not isinstance(value["observedAt"], str)
        or value["domainKey"] != config["domain"]
        or not _number(value["writeWaitP95Milliseconds"], 0)
        or not isinstance(value["waitValid"], bool)
        or not isinstance(value["emergency"], bool)
        or not isinstance(value["managementPlaneHealthy"], bool)
    ):
        raise ValidationError("{} structure is invalid".format(phase))
    if value["waitValid"] != ("waitEvidence" in value):
        raise ValidationError("{} structure is invalid".format(phase))
    if "waitEvidence" in value and not _validate_wait_evidence(value["waitEvidence"]):
        raise ValidationError("{} structure is invalid".format(phase))
    if "ioPressure" in value:
        pressure = value["ioPressure"]
        if not _exact_keys(pressure, ("someAvg10Percent", "fullAvg10Percent")) or not _number(
            pressure["someAvg10Percent"], 0, 100
        ) or not _number(pressure["fullAvg10Percent"], 0, 100):
            raise ValidationError("{} structure is invalid".format(phase))
    if "diskSignals" in value:
        signals = value["diskSignals"]
        signal_keys = (
            "resourceKey",
            "readsCompletedTotal",
            "writesCompletedTotal",
            "readSectorsTotal",
            "writtenSectorsTotal",
            "inFlightIo",
            "ioTimeMillisecondsTotal",
            "weightedIoMillisecondsTotal",
        )
        if not isinstance(signals, list) or len(signals) > 64:
            raise ValidationError("{} structure is invalid".format(phase))
        for disk in signals:
            if not _exact_keys(disk, signal_keys) or disk["resourceKey"] not in config["resources"]:
                raise ValidationError("{} structure is invalid".format(phase))
            for key in signal_keys[1:]:
                if isinstance(disk[key], bool) or not isinstance(disk[key], int) or disk[key] < 0:
                    raise ValidationError("{} structure is invalid".format(phase))


def _read_watch_records(
    binary,
    config_path,
    config,
    identity,
    launch_hook,
    watch_timeout,
    stop_timeout,
):
    phase = "watch"
    process = _spawn(
        binary,
        ("agent", "watch", "--config", str(config_path), "--period", "1s"),
        phase,
        identity,
        launch_hook,
    )
    selector = selectors.DefaultSelector()
    selector.register(process.stdout, selectors.EVENT_READ, "stdout")
    selector.register(process.stderr, selectors.EVENT_READ, "stderr")
    buffers = {"stdout": bytearray(), "stderr": bytearray()}
    records = []
    deadline = time.monotonic() + watch_timeout
    try:
        while len(records) < 2:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                _kill_group(process)
                raise ValidationError("watch timed out before two samples")
            events = selector.select(min(remaining, 0.1))
            if not events and process.poll() is not None:
                _kill_group(process)
                raise ValidationError("watch ended before two samples")
            for key, _mask in events:
                chunk = os.read(key.fileobj.fileno(), 65536)
                if not chunk:
                    selector.unregister(key.fileobj)
                    continue
                target = buffers[key.data]
                target.extend(chunk)
                if len(target) > MAX_OUTPUT_BYTES:
                    _kill_group(process)
                    raise ValidationError("watch output exceeded safety limit")
                if key.data == "stderr" and target.strip():
                    _kill_group(process)
                    raise ValidationError("watch wrote stderr")
                if key.data == "stdout":
                    while b"\n" in target and len(records) < 2:
                        line, _, remainder = target.partition(b"\n")
                        buffers["stdout"] = target = bytearray(remainder)
                        if not line.strip():
                            _kill_group(process)
                            raise ValidationError("watch JSON is invalid")
                        value = _decode_json(bytes(line), "watch")
                        _assert_private_absent(value, config["private"], "watch")
                        _validate_observation(value, config, "watch")
                        records.append(value)
        if buffers["stderr"].strip():
            _kill_group(process)
            raise ValidationError("watch wrote stderr")
        try:
            process.send_signal(signal.SIGTERM)
        except ProcessLookupError:
            raise ValidationError("watch ended before SIGTERM") from None

        stop_deadline = time.monotonic() + stop_timeout
        while selector.get_map():
            remaining = stop_deadline - time.monotonic()
            if remaining <= 0:
                _kill_group(process)
                raise ValidationError("watch did not stop after SIGTERM")
            events = selector.select(min(remaining, 0.1))
            if not events and process.poll() is not None:
                events = [(key, selectors.EVENT_READ) for key in selector.get_map().values()]
            for key, _mask in events:
                chunk = os.read(key.fileobj.fileno(), 65536)
                if not chunk:
                    selector.unregister(key.fileobj)
                    continue
                buffers[key.data].extend(chunk)
                if len(buffers[key.data]) > MAX_OUTPUT_BYTES:
                    _kill_group(process)
                    raise ValidationError("watch output exceeded safety limit")
        try:
            return_code = process.wait(timeout=max(0.0, stop_deadline - time.monotonic()))
        except subprocess.TimeoutExpired:
            _kill_group(process)
            raise ValidationError("watch did not stop after SIGTERM") from None
    finally:
        selector.close()
        if process.poll() is None:
            _kill_group(process)
        _close_pipes(process)
    if buffers["stderr"].strip():
        raise ValidationError("watch wrote stderr")
    if buffers["stdout"].strip():
        raise ValidationError("watch emitted unexpected output after SIGTERM")
    if return_code != 0:
        raise ValidationError("watch exited nonzero after SIGTERM")
    return len(records)


def _trusted_host_executable(path):
    try:
        info = os.lstat(path)
    except OSError:
        return False
    return (
        stat.S_ISREG(info.st_mode)
        and not stat.S_ISLNK(info.st_mode)
        and info.st_uid == 0
        and not stat.S_IMODE(info.st_mode) & 0o022
        and bool(stat.S_IMODE(info.st_mode) & 0o111)
    )


def _default_host_probe():
    return (
        os.path.isdir("/etc/pve")
        and _trusted_host_executable("/usr/bin/pvesh")
        and _trusted_host_executable("/usr/sbin/zpool")
    )


def validate(
    binary_path,
    config_path,
    expected_digest,
    *,
    effective_uid=None,
    platform=None,
    host_probe=None,
    launch_hook=None,
    one_shot_timeout=None,
    watch_timeout=None,
    stop_timeout=10,
):
    effective_uid = os.geteuid() if effective_uid is None else effective_uid
    platform = sys.platform if platform is None else platform
    host_probe = _default_host_probe if host_probe is None else host_probe
    if effective_uid == 0:
        raise ValidationError("root execution is forbidden")
    if platform != "linux" or not host_probe():
        raise ValidationError("PVE host prerequisites are unavailable")
    binary = Path(binary_path)
    config_path = Path(config_path)
    config = _read_private_config(config_path, effective_uid)
    identity = _binary_identity(binary, effective_uid, expected_digest)
    if one_shot_timeout is None:
        one_shot_timeout = max(30, min(600, 6 * config["command_timeout"] + 5))
    if watch_timeout is None:
        watch_timeout = max(60, min(1200, 2 * one_shot_timeout + 10))

    version_payload = _run_one_shot(binary, ("version",), "version", identity, launch_hook, one_shot_timeout)
    try:
        binary_version = version_payload.decode("ascii").strip()
    except UnicodeDecodeError:
        raise ValidationError("version output is invalid") from None
    if not VERSION.fullmatch(binary_version) or len(version_payload.splitlines()) != 1:
        raise ValidationError("version output is invalid")

    inventory_payload = _run_one_shot(
        binary,
        ("agent", "inventory", "--config", str(config_path)),
        "inventory",
        identity,
        launch_hook,
        one_shot_timeout,
    )
    inventory = _decode_json(inventory_payload, "inventory")
    _assert_private_absent(inventory, config["private"], "inventory")
    _validate_inventory(inventory, config)

    observation_payload = _run_one_shot(
        binary,
        ("agent", "observe", "--config", str(config_path)),
        "observe",
        identity,
        launch_hook,
        one_shot_timeout,
    )
    observation = _decode_json(observation_payload, "observe")
    _assert_private_absent(observation, config["private"], "observe")
    _validate_observation(observation, config, "observe")

    watch_samples = _read_watch_records(
        binary,
        config_path,
        config,
        identity,
        launch_hook,
        watch_timeout,
        stop_timeout,
    )
    return {
        "schemaVersion": SCHEMA_VERSION,
        "kind": "PVEHostObserverValidation",
        "validatorVersion": VALIDATOR_VERSION,
        "evidenceScope": "non-production-read-only",
        "binarySha256": identity[6],
        "binaryVersion": binary_version,
        "platformClass": "pve-openzfs-host",
        "checks": {
            "hostPlatformVerified": True,
            "nonRoot": True,
            "configOwnerOnly": True,
            "binaryDigestMatch": True,
            "inventoryValid": True,
            "observationValid": True,
            "watchSamples": watch_samples,
            "sigtermExitZero": True,
            "privateIdentityLeakDetected": False,
            "rawOutputPersisted": False,
        },
        "requestedMutations": 0,
    }


def _parser():
    parser = argparse.ArgumentParser(
        description="Validate a staged read-only observer on a non-production PVE host."
    )
    parser.add_argument("--binary", required=True, help="absolute trusted observer binary path")
    parser.add_argument("--config", required=True, help="absolute owner-only private config path")
    parser.add_argument("--expected-sha256", required=True, help="approved sha256:<hex> binary digest")
    return parser


def main(argv=None, **dependencies):
    arguments = _parser().parse_args(argv)
    try:
        result = validate(
            Path(arguments.binary),
            Path(arguments.config),
            arguments.expected_sha256,
            **dependencies,
        )
    except ValidationError as error:
        print("host validation: {}".format(error), file=sys.stderr)
        return 1
    print(json.dumps(result, sort_keys=True, separators=(",", ":")))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
