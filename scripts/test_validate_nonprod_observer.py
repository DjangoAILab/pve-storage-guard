import contextlib
import hashlib
import io
import json
import os
from pathlib import Path
import re
import stat
import tempfile
import textwrap
import unittest

from scripts import validate_nonprod_observer as validator


CONFIG = {
    "apiVersion": "guard.storage-slo.io/v1alpha1",
    "kind": "PVEAgentConfig",
    "spec": {
        "domainKey": "reference-pool",
        "node": "private-node",
        "storage": "private-storage",
        "zpool": "private-pool",
        "sampleIntervalSeconds": 1,
        "commandTimeoutSeconds": 5,
        "emergencyWaitMilliseconds": 100,
        "resources": [
            {
                "resourceKey": "resource-a",
                "kernelDevice": "private-device",
                "root": False,
                "critical": False,
            }
        ],
    },
}


FAKE_BINARY = r'''#!/usr/bin/env python3
import json
import signal
import sys
import time

MODE = %r
args = sys.argv[1:]

inventory = {
    "schemaVersion": "guard.storage-slo.io/v1alpha1",
    "kind": "PVEInventory",
    "observedAt": "2026-08-02T00:00:00Z",
    "domainKey": "reference-pool",
    "storageType": "zfspool",
    "resources": [{"resourceKey": "resource-a", "root": False, "critical": False}],
}
observation = {
    "schemaVersion": "guard.storage-slo.io/v1alpha1",
    "id": "observation-opaque",
    "observedAt": "2026-08-02T00:00:01Z",
    "domainKey": "reference-pool",
    "writeWaitP95Milliseconds": 2.097151,
    "waitValid": True,
    "emergency": False,
    "managementPlaneHealthy": True,
    "waitEvidence": {
        "measurementLayer": "storage-domain",
        "statistic": "p95-upper-bound",
        "source": "openzfs-total-wait-histogram",
        "provenance": "observed",
        "sampleIntervalSeconds": 1,
        "sampleWeight": 100,
        "bucketUpperBoundNanoseconds": 2097151,
    },
    "ioPressure": {"someAvg10Percent": 2.5, "fullAvg10Percent": 0.25},
    "diskSignals": [{
        "resourceKey": "resource-a",
        "readsCompletedTotal": 1,
        "writesCompletedTotal": 2,
        "readSectorsTotal": 3,
        "writtenSectorsTotal": 4,
        "inFlightIo": 0,
        "ioTimeMillisecondsTotal": 5,
        "weightedIoMillisecondsTotal": 6,
    }],
}

if args == ["version"]:
    if MODE == "timeout":
        time.sleep(30)
    if MODE == "oversized":
        print("x" * (1024 * 1024 + 1))
    else:
        print("v0.1.0-test")
    raise SystemExit(0)

if len(args) < 4 or args[0] != "agent" or args[2] != "--config":
    raise SystemExit(64)
operation = args[1]

if operation == "inventory" and len(args) == 4:
    if MODE == "duplicate":
        print('{"schemaVersion":"guard.storage-slo.io/v1alpha1","schemaVersion":"duplicate"}')
    elif MODE == "private":
        inventory["domainKey"] = "private-node"
        print(json.dumps(inventory, separators=(",", ":")))
    elif MODE == "wrong-domain":
        inventory["domainKey"] = "other-pool"
        print(json.dumps(inventory, separators=(",", ":")))
    else:
        print(json.dumps(inventory, separators=(",", ":")))
    raise SystemExit(0)

if operation == "observe" and len(args) == 4:
    if MODE == "observe-stderr":
        print("private-node failed", file=sys.stderr)
        raise SystemExit(7)
    print(json.dumps(observation, separators=(",", ":")))
    raise SystemExit(0)

if operation != "watch" or len(args) != 6 or args != ["agent", "watch", "--config", args[3], "--period", "1s"]:
    raise SystemExit(64)

def raise_exit(code):
    raise SystemExit(code)

if MODE == "watch-ignore":
    signal.signal(signal.SIGTERM, signal.SIG_IGN)
elif MODE == "watch-nonzero":
    signal.signal(signal.SIGTERM, lambda *_: raise_exit(3))
else:
    signal.signal(signal.SIGTERM, lambda *_: raise_exit(0))

def emit(value):
    print(json.dumps(value, separators=(",", ":")), flush=True)

emit(observation)
if MODE != "watch-one":
    emit(observation)
while True:
    time.sleep(1)
'''


class NonProductionObserverValidatorTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.test_uid = os.geteuid() if os.geteuid() != 0 else 1000
        self.config_path = self.root / "agent.json"
        self.config_path.write_text(json.dumps(CONFIG), encoding="utf-8")
        self.config_path.chmod(0o600)
        if os.geteuid() == 0:
            os.chown(self.config_path, self.test_uid, -1)

    def write_binary(self, mode="normal"):
        path = self.root / "pve-storage-guard"
        path.write_text(textwrap.dedent(FAKE_BINARY % mode), encoding="utf-8")
        path.chmod(0o755)
        if os.geteuid() == 0:
            os.chown(path, self.test_uid, -1)
        return path

    @staticmethod
    def digest(path):
        return "sha256:" + hashlib.sha256(path.read_bytes()).hexdigest()

    def validate(self, binary, **overrides):
        options = {
            "effective_uid": self.test_uid,
            "platform": "linux",
            "host_probe": lambda: True,
            "one_shot_timeout": 10.0,
            "watch_timeout": 10.0,
            "stop_timeout": 0.5,
        }
        options.update(overrides)
        return validator.validate(
            binary,
            self.config_path,
            self.digest(binary),
            **options,
        )

    def assert_category(self, category, mode="normal", **overrides):
        binary = self.write_binary(mode)
        with self.assertRaises(validator.ValidationError) as caught:
            self.validate(binary, **overrides)
        message = str(caught.exception)
        self.assertEqual(message, category)
        for private in ("private-node", "private-storage", "private-pool", "private-device"):
            self.assertNotIn(private, message)

    def test_success_emits_identity_free_bounded_summary(self):
        binary = self.write_binary()
        before = sorted(path.name for path in self.root.iterdir())
        result = self.validate(binary)
        after = sorted(path.name for path in self.root.iterdir())
        self.assertEqual(before, after)
        self.assertEqual(result["kind"], "PVEHostObserverValidation")
        self.assertEqual(result["validatorVersion"], "v1alpha1")
        self.assertEqual(result["binarySha256"], self.digest(binary))
        self.assertEqual(result["requestedMutations"], 0)
        self.assertEqual(result["checks"]["watchSamples"], 2)
        self.assertTrue(result["checks"]["sigtermExitZero"])
        self.assertFalse(result["checks"]["privateIdentityLeakDetected"])
        self.assertEqual(
            set(result),
            {
                "schemaVersion",
                "kind",
                "validatorVersion",
                "evidenceScope",
                "binarySha256",
                "binaryVersion",
                "platformClass",
                "checks",
                "requestedMutations",
            },
        )
        schema_path = Path(__file__).parents[1] / "api/v1/schema/pve-host-observer-validation.schema.json"
        schema = json.loads(schema_path.read_text(encoding="utf-8"))
        self.assertEqual(set(result), set(schema["required"]))
        for key, definition in schema["properties"].items():
            if "const" in definition:
                self.assertEqual(result[key], definition["const"])
            if "pattern" in definition:
                self.assertRegex(result[key], re.compile(definition["pattern"]))
        checks_schema = schema["properties"]["checks"]
        self.assertEqual(set(result["checks"]), set(checks_schema["required"]))
        for key, definition in checks_schema["properties"].items():
            self.assertEqual(result["checks"][key], definition["const"])
        serialized = json.dumps(result, sort_keys=True)
        for forbidden in (
            *CONFIG["spec"].values(),
            str(binary),
            str(self.config_path),
            "resource-a",
            "reference-pool",
            "observation-opaque",
            "2.097151",
        ):
            if isinstance(forbidden, str):
                self.assertNotIn(forbidden, serialized)

    def test_rejects_root_before_launch(self):
        self.assert_category("root execution is forbidden", effective_uid=0)

    def test_rejects_non_linux_or_missing_pve_prerequisites(self):
        self.assert_category("PVE host prerequisites are unavailable", platform="darwin")
        self.assert_category("PVE host prerequisites are unavailable", host_probe=lambda: False)

    def test_rejects_unsafe_config_and_binary_files(self):
        binary = self.write_binary()
        self.config_path.chmod(0o640)
        with self.assertRaisesRegex(validator.ValidationError, "config permissions are unsafe"):
            self.validate(binary)
        self.config_path.chmod(0o600)
        link = self.root / "config-link.json"
        link.symlink_to(self.config_path)
        with self.assertRaisesRegex(validator.ValidationError, "config file is unsafe"):
            validator.validate(
                binary,
                link,
                self.digest(binary),
                effective_uid=self.test_uid,
                platform="linux",
                host_probe=lambda: True,
            )
        binary.chmod(0o775)
        with self.assertRaisesRegex(validator.ValidationError, "binary permissions are unsafe"):
            self.validate(binary)
        binary.unlink()
        target = self.write_binary()
        binary_link = self.root / "binary-link"
        binary_link.symlink_to(target)
        with self.assertRaisesRegex(validator.ValidationError, "binary file is unsafe"):
            validator.validate(
                binary_link,
                self.config_path,
                self.digest(target),
                effective_uid=self.test_uid,
                platform="linux",
                host_probe=lambda: True,
            )

    def test_rejects_wrong_digest_and_binary_replacement(self):
        binary = self.write_binary()
        with self.assertRaisesRegex(validator.ValidationError, "expected binary digest is invalid"):
            validator.validate(
                binary,
                self.config_path,
                "invalid",
                effective_uid=self.test_uid,
                platform="linux",
                host_probe=lambda: True,
            )
        with self.assertRaisesRegex(validator.ValidationError, "binary digest mismatch"):
            validator.validate(
                binary,
                self.config_path,
                "sha256:" + "0" * 64,
                effective_uid=self.test_uid,
                platform="linux",
                host_probe=lambda: True,
            )

        replaced = False

        def replace_once(_phase, path):
            nonlocal replaced
            if replaced:
                return
            replacement = self.root / "replacement"
            replacement.write_text("replacement", encoding="utf-8")
            replacement.chmod(0o755)
            os.replace(replacement, path)
            replaced = True

        with self.assertRaisesRegex(validator.ValidationError, "binary changed during validation"):
            self.validate(binary, launch_hook=replace_once)

    def test_rejects_unknown_or_duplicate_config_fields(self):
        binary = self.write_binary()
        document = dict(CONFIG)
        document["unexpected"] = True
        self.config_path.write_text(json.dumps(document), encoding="utf-8")
        self.assert_category("config structure is invalid")
        self.config_path.write_text(
            '{"apiVersion":"guard.storage-slo.io/v1alpha1","apiVersion":"duplicate","kind":"PVEAgentConfig","spec":{}}',
            encoding="utf-8",
        )
        self.assert_category("config JSON is invalid")

    def test_rejects_timeout_overflow_duplicate_output_and_private_leak(self):
        self.assert_category("version timed out", mode="timeout", one_shot_timeout=0.2)
        self.assert_category("version output exceeded safety limit", mode="oversized")
        self.assert_category("inventory JSON is invalid", mode="duplicate")
        self.assert_category("private identity appeared in inventory", mode="private")

    def test_rejects_child_stderr_and_structural_mismatch(self):
        self.assert_category("observe wrote stderr", mode="observe-stderr")
        self.assert_category("inventory structure is invalid", mode="wrong-domain")

    def test_rejects_watch_shortfall_ignored_signal_and_nonzero_exit(self):
        self.assert_category("watch timed out before two samples", mode="watch-one", watch_timeout=0.3)
        self.assert_category("watch did not stop after SIGTERM", mode="watch-ignore")
        self.assert_category("watch exited nonzero after SIGTERM", mode="watch-nonzero")

    def test_cli_failure_has_zero_stdout_and_categorical_stderr(self):
        binary = self.write_binary("private")
        stdout = io.StringIO()
        stderr = io.StringIO()
        with contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
            exit_code = validator.main(
                [
                    "--binary",
                    str(binary),
                    "--config",
                    str(self.config_path),
                    "--expected-sha256",
                    self.digest(binary),
                ],
                effective_uid=self.test_uid,
                platform="linux",
                host_probe=lambda: True,
                one_shot_timeout=10.0,
                watch_timeout=10.0,
                stop_timeout=0.5,
            )
        self.assertEqual(exit_code, 1)
        self.assertEqual(stdout.getvalue(), "")
        self.assertEqual(stderr.getvalue(), "host validation: private identity appeared in inventory\n")

    def test_cli_success_is_one_json_line_with_zero_stderr(self):
        binary = self.write_binary()
        stdout = io.StringIO()
        stderr = io.StringIO()
        with contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
            exit_code = validator.main(
                [
                    "--binary",
                    str(binary),
                    "--config",
                    str(self.config_path),
                    "--expected-sha256",
                    self.digest(binary),
                ],
                effective_uid=self.test_uid,
                platform="linux",
                host_probe=lambda: True,
                one_shot_timeout=10.0,
                watch_timeout=10.0,
                stop_timeout=0.5,
            )
        self.assertEqual(exit_code, 0)
        self.assertEqual(stderr.getvalue(), "")
        lines = stdout.getvalue().splitlines()
        self.assertEqual(len(lines), 1)
        self.assertEqual(json.loads(lines[0])["validatorVersion"], "v1alpha1")

    @unittest.skipUnless(os.environ.get("PVE_STORAGE_GUARD_TEST_BINARY"), "compiled project binary not supplied")
    def test_compiled_project_binary_failure_is_categorical(self):
        if os.geteuid() == 0:
            self.skipTest("non-root runner required")
        binary = Path(os.environ["PVE_STORAGE_GUARD_TEST_BINARY"])
        with self.assertRaises(validator.ValidationError) as caught:
            validator.validate(
                binary,
                self.config_path,
                self.digest(binary),
                effective_uid=os.geteuid(),
                platform="linux",
                host_probe=lambda: True,
                one_shot_timeout=10,
                watch_timeout=1,
                stop_timeout=1,
            )
        self.assertIn(str(caught.exception), {"inventory wrote stderr", "inventory failed"})
        self.assertNotIn("pvesh", str(caught.exception))


if __name__ == "__main__":
    unittest.main()
