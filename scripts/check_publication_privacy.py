#!/usr/bin/env python3
"""Fail closed when authored publication surfaces expose private coordinates."""

from __future__ import annotations

import hashlib
import ipaddress
import os
from pathlib import Path, PurePosixPath
import re
import subprocess
import sys
from dataclasses import dataclass
from urllib.parse import urlsplit


ALLOWED_URL_HOSTS = frozenset(
    {
        "djangoailab.github.io",
        "docs.kernel.org",
        "github.com",
        "guard.storage-slo.io",
        "json-schema.org",
        "openzfs.github.io",
        "pve.proxmox.com",
        "skuld.cs.umass.edu",
        "traces.cs.umass.edu",
        "www.contributor-covenant.org",
        "www.qemu.org",
        "www.snia.org",
        "www.w3.org",
    }
)

URL_RE = re.compile(r"https?://[^\s<>\"']+", re.IGNORECASE)
IPV4_RE = re.compile(
    r"(?<![0-9.])(?:[0-9]{1,3}\.){3}[0-9]{1,3}(?=$|[^0-9.]|\.(?=\s|$))"
)
TRAILING_URL_PUNCTUATION = ".,;:!?)]}"
LOCAL_HOST_SUFFIXES = (".localhost", ".local", ".internal", ".lan", ".home", ".home.arpa")
RFC1918_NETWORKS = tuple(
    ipaddress.ip_network(cidr) for cidr in ("10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16")
)


@dataclass(frozen=True, order=True)
class Finding:
    path: str
    line: int
    category: str
    fingerprint: str

    def render(self) -> str:
        return f"{self.path}:{self.line}: {self.category} [sha256:{self.fingerprint}]"


def fingerprint(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()[:12]


def is_publication_surface(path: str) -> bool:
    candidate = PurePosixPath(path)
    suffix = candidate.suffix.lower()
    parts = candidate.parts

    if len(parts) == 1 and suffix in {".md", ".mdx"}:
        return True
    if parts[:1] == ("docs",) and suffix in {".md", ".mdx", ".svg"}:
        return True
    if parts[:3] == ("website", "src", "content") and suffix in {".md", ".mdx"}:
        return True
    if parts[:2] == ("website", "public") and suffix == ".svg":
        return True
    if (
        parts[:3] == ("api", "v1", "schema")
        or parts[:2] == ("configs", "examples")
        or parts[:2] == ("poc", "schema")
    ) and suffix == ".json":
        return True
    if parts[:1] == (".github",) and suffix in {".md", ".mdx", ".yml", ".yaml"}:
        return True
    return False


def _normalize_url_token(token: str) -> str:
    return token.rstrip(TRAILING_URL_PUNCTUATION)


def _url_finding(path: str, line_number: int, token: str) -> Finding | None:
    url = _normalize_url_token(token)
    try:
        parsed = urlsplit(url)
        host = parsed.hostname
        # Accessing port validates malformed bracket and port syntax.
        _ = parsed.port
    except ValueError:
        return Finding(path, line_number, "malformed-public-url", fingerprint(url))

    if parsed.username is not None or parsed.password is not None:
        return Finding(path, line_number, "url-userinfo", fingerprint(parsed.netloc))
    if not host:
        return Finding(path, line_number, "malformed-public-url", fingerprint(url))

    normalized_host = host.lower()
    try:
        address = ipaddress.ip_address(normalized_host)
    except ValueError:
        address = None

    if address is not None:
        category = "private-url-ip" if not address.is_global else "unapproved-url-ip"
        return Finding(path, line_number, category, fingerprint(normalized_host))

    if (
        normalized_host == "localhost"
        or "." not in normalized_host
        or normalized_host.endswith(LOCAL_HOST_SUFFIXES)
    ):
        return Finding(path, line_number, "local-url-host", fingerprint(normalized_host))
    if normalized_host not in ALLOWED_URL_HOSTS:
        return Finding(path, line_number, "unapproved-url-host", fingerprint(normalized_host))
    return None


def scan_text(path: str, text: str) -> tuple[list[Finding], int]:
    findings: list[Finding] = []
    url_count = 0
    for line_number, line in enumerate(text.splitlines(), start=1):
        url_spans: list[tuple[int, int]] = []
        for match in URL_RE.finditer(line):
            url_count += 1
            url_spans.append(match.span())
            finding = _url_finding(path, line_number, match.group(0))
            if finding is not None:
                findings.append(finding)

        for match in IPV4_RE.finditer(line):
            if any(start <= match.start() < end for start, end in url_spans):
                continue
            try:
                address = ipaddress.ip_address(match.group(0))
            except ValueError:
                continue
            if any(address in network for network in RFC1918_NETWORKS):
                findings.append(
                    Finding(path, line_number, "private-ipv4", fingerprint(str(address)))
                )
    return findings, url_count


def tracked_entries(root: Path) -> list[tuple[str, str]]:
    result = subprocess.run(
        ["git", "ls-files", "--stage", "-z"],
        cwd=root,
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    entries: list[tuple[str, str]] = []
    for record in result.stdout.split(b"\0"):
        if not record:
            continue
        metadata, raw_path = record.split(b"\t", 1)
        mode = metadata.split(b" ", 1)[0].decode("ascii")
        path = raw_path.decode("utf-8", errors="strict")
        entries.append((mode, path))
    return entries


def scan_repository(root: Path) -> tuple[list[Finding], int, int]:
    findings: list[Finding] = []
    file_count = 0
    url_count = 0
    for mode, relative_path in tracked_entries(root):
        if not is_publication_surface(relative_path):
            continue
        file_count += 1
        if mode == "120000":
            findings.append(
                Finding(relative_path, 1, "publication-symlink", fingerprint(relative_path))
            )
            continue
        try:
            text = (root / relative_path).read_text(encoding="utf-8", errors="strict")
        except (OSError, UnicodeError) as error:
            findings.append(
                Finding(
                    relative_path,
                    1,
                    "unreadable-publication-file",
                    fingerprint(type(error).__name__),
                )
            )
            continue
        file_findings, urls = scan_text(relative_path, text)
        findings.extend(file_findings)
        url_count += urls
    return sorted(set(findings)), file_count, url_count


def repository_root() -> Path:
    result = subprocess.run(
        ["git", "rev-parse", "--show-toplevel"],
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    return Path(result.stdout.strip()).resolve()


def main() -> int:
    os.environ.setdefault("LC_ALL", "C.UTF-8")
    try:
        findings, file_count, url_count = scan_repository(repository_root())
    except (OSError, subprocess.SubprocessError, UnicodeError) as error:
        print(
            "publication privacy scan failed closed: "
            f"scanner-error [sha256:{fingerprint(type(error).__name__)}]",
            file=sys.stderr,
        )
        return 2

    if findings:
        print(f"publication privacy scan rejected {len(findings)} finding(s):", file=sys.stderr)
        for finding in findings:
            print(f"  {finding.render()}", file=sys.stderr)
        return 1

    print(f"publication privacy scan passed: files={file_count} urls={url_count}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
