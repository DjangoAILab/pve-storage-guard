#!/usr/bin/env python3
from __future__ import annotations

import json
from pathlib import Path
import unittest

from scripts import rehearse_systemd_lifecycle as rehearsal


class SystemdLifecycleRehearsalTests(unittest.TestCase):
    def test_config_is_synthetic_and_binds_selected_device(self) -> None:
        document = json.loads(rehearsal._config_payload("rehearsal-baseline", "test-device"))
        self.assertEqual(rehearsal.SCHEMA_VERSION, document["apiVersion"])
        self.assertEqual("PVEAgentConfig", document["kind"])
        self.assertEqual("rehearsal-baseline", document["spec"]["domainKey"])
        self.assertEqual("fixture-node", document["spec"]["node"])
        self.assertEqual("fixture-storage", document["spec"]["storage"])
        self.assertEqual("fixturepool", document["spec"]["zpool"])
        self.assertEqual("test-device", document["spec"]["resources"][0]["kernelDevice"])

    def test_shims_accept_only_fixed_read_operations(self) -> None:
        for payload in (rehearsal._pvesh_shim(), rehearsal._zpool_shim()):
            text = payload.decode("ascii")
            self.assertIn("*) exit 64 ;;", text)
            self.assertNotIn("eval", text)
            self.assertNotIn("sudo", text)
            self.assertNotIn("curl", text)
        self.assertIn("get /cluster/status --output-format json", rehearsal._pvesh_shim().decode("ascii"))
        self.assertIn("iostat -wpH -y fixturepool 1 1", rehearsal._zpool_shim().decode("ascii"))

    def test_validates_strict_synthetic_observation(self) -> None:
        observation = {
            "schemaVersion": rehearsal.SCHEMA_VERSION,
            "id": "observation-0123456789abcdef01234567",
            "observedAt": "2026-08-03T00:00:00Z",
            "domainKey": "rehearsal-baseline",
            "writeWaitP95Milliseconds": 2.097151,
            "waitValid": True,
            "emergency": False,
            "managementPlaneHealthy": True,
            "diskSignals": [{"resourceKey": "fixture-resource"}],
        }
        rehearsal._validate_observation(observation, "rehearsal-baseline")
        for mutation in (
            {**observation, "domainKey": "different"},
            {**observation, "unexpected": True},
            {**observation, "managementPlaneHealthy": False},
            {**observation, "diskSignals": []},
        ):
            with self.assertRaises(rehearsal.RehearsalError):
                rehearsal._validate_observation(mutation, "rehearsal-baseline")

    def test_inventory_record_is_not_misclassified_as_watch_sample(self) -> None:
        inventory = {
            "schemaVersion": rehearsal.SCHEMA_VERSION,
            "kind": "PVEInventory",
            "domainKey": "rehearsal-baseline",
            "resources": [],
        }
        self.assertIsNone(
            rehearsal._new_watch_identifier(inventory, "rehearsal-baseline", set())
        )

    def test_preflight_precedes_first_mutation(self) -> None:
        source = Path(rehearsal.__file__).read_text(encoding="utf-8")
        body = source.split("def rehearse(", 1)[1].split("def _parser", 1)[0]
        self.assertLess(body.index("_preflight("), body.index("_install_runtime("))
        preflight = source.split("def _preflight(", 1)[1].split("def _fsync_directory", 1)[0]
        self.assertIn("any(_lexists(path) for path in MUTATION_TARGETS)", preflight)
        self.assertIn(rehearsal.PVE_DIRECTORY, rehearsal.MUTATION_TARGETS)

    def test_cleanup_targets_are_exact_and_bounded(self) -> None:
        targets = {str(path) for path in rehearsal.MUTATION_TARGETS}
        self.assertNotIn("/", targets)
        self.assertNotIn("/etc", targets)
        self.assertNotIn("/usr", targets)
        self.assertIn("/etc/pve", targets)
        self.assertIn("/usr/local/bin/pve-storage-guard", targets)
        self.assertIn("/run/systemd/system/pve-storage-guard-observer.service", targets)

    def test_parent_directory_gate_rejects_missing_path(self) -> None:
        self.assertFalse(rehearsal._trusted_root_directory(Path("/definitely-not-present-pve-guard")))


if __name__ == "__main__":
    unittest.main()
