from __future__ import annotations

import io
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock

from scripts import check_publication_privacy as privacy


class PublicationSurfaceTests(unittest.TestCase):
    def test_selects_only_authored_publication_surfaces(self) -> None:
        selected = {
            "README.md",
            "docs/ARCHITECTURE.md",
            "docs/assets/chart.svg",
            "website/src/content/docs/index.mdx",
            "website/public/chart.svg",
            "api/v1/schema/event.json",
            "configs/examples/policy.json",
            "poc/schema/trace.json",
            ".github/workflows/ci.yml",
        }
        excluded = {
            "website/package-lock.json",
            "website/dist/index.html",
            "internal/policy/policy.go",
            "poc/results/report.json",
            "scripts/example.md",
        }
        self.assertTrue(all(privacy.is_publication_surface(path) for path in selected))
        self.assertTrue(all(not privacy.is_publication_surface(path) for path in excluded))


class ScannerTests(unittest.TestCase):
    def test_accepts_exactly_allowlisted_public_url(self) -> None:
        findings, count = privacy.scan_text(
            "README.md", "See https://github.com/DjangoAILab/pve-storage-guard."
        )
        self.assertEqual([], findings)
        self.assertEqual(1, count)

    def test_exact_host_allowlist_does_not_accept_suffix_trick(self) -> None:
        findings, _ = privacy.scan_text(
            "README.md", "See https://github.com.example.invalid/project"
        )
        self.assertEqual("unapproved-url-host", findings[0].category)

    def test_rejects_private_ipv4_in_prose(self) -> None:
        findings, _ = privacy.scan_text("docs/runbook.md", "Connect to 192.168.7.4.")
        self.assertEqual(["private-ipv4"], [finding.category for finding in findings])

    def test_rejects_private_and_loopback_url_literals(self) -> None:
        private, _ = privacy.scan_text("README.md", "http://10.2.3.4/status")
        loopback, _ = privacy.scan_text("README.md", "http://127.0.0.1/status")
        self.assertEqual("private-url-ip", private[0].category)
        self.assertEqual("private-url-ip", loopback[0].category)

    def test_rejects_url_userinfo(self) -> None:
        findings, _ = privacy.scan_text(
            "README.md", "https://operator:secret@github.com/example/project"
        )
        self.assertEqual("url-userinfo", findings[0].category)

    def test_rendered_failure_never_contains_rejected_coordinate(self) -> None:
        secret_host = "private-control.example.invalid"
        secret_ip = "172.20.4.9"
        findings, _ = privacy.scan_text(
            "docs/runbook.md", f"https://{secret_host}/status and {secret_ip}"
        )
        output = "\n".join(finding.render() for finding in findings)
        self.assertNotIn(secret_host, output)
        self.assertNotIn(secret_ip, output)
        self.assertIn("sha256:", output)

    def test_repository_scan_reports_aggregate_for_valid_tree(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            subprocess.run(["git", "init", "-q"], cwd=root, check=True)
            (root / "README.md").write_text(
                "See https://docs.kernel.org/admin-guide/iostats.html\n", encoding="utf-8"
            )
            (root / "package-lock.json").write_text(
                '{"url":"https://registry.invalid/dependency"}\n', encoding="utf-8"
            )
            subprocess.run(
                ["git", "add", "README.md", "package-lock.json"], cwd=root, check=True
            )
            findings, files, urls = privacy.scan_repository(root)
        self.assertEqual([], findings)
        self.assertEqual(1, files)
        self.assertEqual(1, urls)

    def test_main_redacts_failure_output(self) -> None:
        hidden = "sensitive-host.example.invalid"
        finding = privacy.Finding(
            "README.md", 3, "unapproved-url-host", privacy.fingerprint(hidden)
        )
        stderr = io.StringIO()
        with mock.patch.object(
            privacy, "scan_repository", return_value=([finding], 1, 1)
        ), mock.patch.object(privacy, "repository_root", return_value=Path(".")), mock.patch(
            "sys.stderr", stderr
        ):
            self.assertEqual(1, privacy.main())
        self.assertNotIn(hidden, stderr.getvalue())
        self.assertIn("README.md:3", stderr.getvalue())


if __name__ == "__main__":
    unittest.main()
