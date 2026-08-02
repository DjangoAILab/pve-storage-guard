import contextlib
import io
import json
import os
from pathlib import Path
import re
import tempfile
import textwrap
import unittest
from unittest import mock

from scripts import validate_prod_observer_compatibility as validator
from scripts.test_validate_nonprod_observer import CONFIG, FAKE_BINARY


class ProductionObserverCompatibilityTests(unittest.TestCase):
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
        import hashlib
        return "sha256:" + hashlib.sha256(path.read_bytes()).hexdigest()

    def validate(self, binary, effective_uid, *, allow_root=False):
        return validator.validate(
            binary,
            self.config_path,
            self.digest(binary),
            effective_uid=effective_uid,
            allow_root=allow_root,
            platform="linux",
            host_probe=lambda: True,
            one_shot_timeout=10.0,
            watch_timeout=10.0,
            stop_timeout=0.5,
        )

    def test_root_execution_is_reported_but_never_promotion_eligible(self):
        binary = self.write_binary()
        with self.assertRaisesRegex(validator.ValidationError, "explicit acknowledgement"):
            self.validate(binary, effective_uid=0)
        evidence = {
            "binarySha256": self.digest(binary),
            "binaryVersion": "v0.1.0-test",
            "nonRoot": False,
            "watchSamples": 2,
        }
        with mock.patch.object(validator.shared, "_collect_observer_evidence", return_value=evidence):
            result = self.validate(binary, effective_uid=0, allow_root=True)

        self.assertEqual(result["kind"], "PVEHostObserverCompatibility")
        self.assertEqual(result["evidenceScope"], "production-read-only-compatibility")
        self.assertFalse(result["promotionEligible"])
        self.assertFalse(result["checks"]["nonRoot"])
        self.assertIn("non-root-not-validated", result["limitations"])
        self.assertEqual(result["requestedMutations"], 0)

    def test_non_root_compatibility_remains_ineligible(self):
        binary = self.write_binary()
        result = self.validate(binary, effective_uid=self.test_uid)
        self.assertTrue(result["checks"]["nonRoot"])
        self.assertFalse(result["promotionEligible"])
        self.assertNotIn("non-root-not-validated", result["limitations"])

    def test_private_identity_failure_is_categorical(self):
        binary = self.write_binary("private")
        with self.assertRaisesRegex(validator.ValidationError, "private identity appeared in inventory") as caught:
            self.validate(binary, effective_uid=self.test_uid)
        self.assertNotIn("private-node", str(caught.exception))

    def test_success_matches_strict_public_schema_and_is_identity_free(self):
        binary = self.write_binary()
        result = self.validate(binary, effective_uid=self.test_uid)
        schema_path = Path(__file__).parents[1] / "api/v1/schema/pve-host-observer-compatibility.schema.json"
        schema = json.loads(schema_path.read_text(encoding="utf-8"))

        self.assertEqual(set(result), set(schema["required"]))
        for key, definition in schema["properties"].items():
            if "const" in definition:
                self.assertEqual(result[key], definition["const"])
            if "pattern" in definition:
                self.assertRegex(result[key], re.compile(definition["pattern"]))
        self.assertEqual(set(result["checks"]), set(schema["properties"]["checks"]["required"]))
        serialized = json.dumps(result, sort_keys=True)
        for forbidden in ("private-node", "private-storage", "private-pool", "private-device", "resource-a", str(binary), str(self.config_path)):
            self.assertNotIn(forbidden, serialized)

    def test_cli_failure_has_zero_stdout(self):
        binary = self.write_binary("private")
        stdout = io.StringIO()
        stderr = io.StringIO()
        with contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
            code = validator.main(
                ["--binary", str(binary), "--config", str(self.config_path), "--expected-sha256", self.digest(binary)],
                effective_uid=self.test_uid,
                platform="linux",
                host_probe=lambda: True,
                one_shot_timeout=10.0,
                watch_timeout=10.0,
                stop_timeout=0.5,
            )
        self.assertEqual(code, 1)
        self.assertEqual(stdout.getvalue(), "")
        self.assertEqual(stderr.getvalue(), "host compatibility: private identity appeared in inventory\n")


if __name__ == "__main__":
    unittest.main()
