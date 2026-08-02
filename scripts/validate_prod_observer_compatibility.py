#!/usr/bin/env python3
"""Validate compiled observer compatibility on a production PVE host.

This validator is deliberately not a promotion gate. It runs the same bounded,
read-only observer commands as the non-production validator and records root
execution as a limitation instead of presenting it as hardened evidence.
"""

import argparse
import json
from pathlib import Path

if __package__:
    from . import validate_nonprod_observer as shared
else:  # Direct execution when both reviewed scripts are staged together.
    import validate_nonprod_observer as shared


ValidationError = shared.ValidationError


def validate(
    binary_path,
    config_path,
    expected_digest,
    *,
    allow_root=False,
    effective_uid=None,
    platform=None,
    host_probe=None,
    launch_hook=None,
    one_shot_timeout=None,
    watch_timeout=None,
    stop_timeout=10,
):
    observed_uid = shared.os.geteuid() if effective_uid is None else effective_uid
    if observed_uid == 0 and not allow_root:
        raise ValidationError("root compatibility requires explicit acknowledgement")
    evidence = shared._collect_observer_evidence(
        binary_path,
        config_path,
        expected_digest,
        require_non_root=False,
        effective_uid=observed_uid,
        platform=platform,
        host_probe=host_probe,
        launch_hook=launch_hook,
        one_shot_timeout=one_shot_timeout,
        watch_timeout=watch_timeout,
        stop_timeout=stop_timeout,
    )
    limitations = [
        "service-isolation-not-validated",
        "controlled-load-not-validated",
        "actuation-not-validated",
    ]
    if not evidence["nonRoot"]:
        limitations.insert(0, "non-root-not-validated")
    return {
        "schemaVersion": shared.SCHEMA_VERSION,
        "kind": "PVEHostObserverCompatibility",
        "validatorVersion": shared.VALIDATOR_VERSION,
        "evidenceScope": "production-read-only-compatibility",
        "binarySha256": evidence["binarySha256"],
        "binaryVersion": evidence["binaryVersion"],
        "platformClass": "pve-openzfs-host",
        "checks": {
            "hostPlatformVerified": True,
            "nonRoot": evidence["nonRoot"],
            "configOwnerOnly": True,
            "binaryDigestMatch": True,
            "inventoryValid": True,
            "observationValid": True,
            "watchSamples": evidence["watchSamples"],
            "sigtermExitZero": True,
            "privateIdentityLeakDetected": False,
            "rawOutputPersisted": False,
        },
        "promotionEligible": False,
        "limitations": limitations,
        "requestedMutations": 0,
    }


def _parser():
    parser = argparse.ArgumentParser(
        description="Validate read-only observer compatibility on a production PVE host."
    )
    parser.add_argument("--binary", required=True, help="absolute trusted observer binary path")
    parser.add_argument("--config", required=True, help="absolute owner-only private config path")
    parser.add_argument("--expected-sha256", required=True, help="approved sha256:<hex> binary digest")
    parser.add_argument(
        "--allow-root",
        action="store_true",
        help="explicitly acknowledge that reviewed read-only compatibility will run as root",
    )
    return parser


def main(argv=None, **dependencies):
    arguments = _parser().parse_args(argv)
    try:
        result = validate(
            Path(arguments.binary),
            Path(arguments.config),
            arguments.expected_sha256,
            allow_root=arguments.allow_root,
            **dependencies,
        )
    except ValidationError as error:
        print("host compatibility: {}".format(error), file=shared.sys.stderr)
        return 1
    print(json.dumps(result, sort_keys=True, separators=(",", ":")))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
