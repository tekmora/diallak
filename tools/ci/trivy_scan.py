"""Run Trivy in its digest-pinned image, planted cases first.

Two scans. Each first runs on a case planted at run time and must report it;
only then is the real scan trusted.

- fs: the tracked files, for vulnerable Go modules, secrets and Dockerfile
  misconfigurations. The planted tree holds a go.mod that requires a vulnerable
  golang.org/x/text, a Dockerfile that runs as root and an AWS key pair.
- image: an image from the local Docker daemon, for vulnerable OS packages and
  the Go modules built into its binary. The planted image is Alpine 3.12.0,
  pinned by digest, which has fixed vulnerabilities.

The real scan fails on any HIGH or CRITICAL finding that has a fix, on any
Trivy error, and on a report that leaves out what it was given: a tracked
go.mod or Dockerfile (fs), or the image's OS packages or its binary (image).
No .trivyignore is read, so no finding is excepted. With --sarif, the real
scan's report is also converted to SARIF in that file, for upload to code
scanning; the file is written before the scan fails.

Usage:
  python3 -I tools/ci/trivy_scan.py --trivy NAME:TAG@sha256:DIGEST [--sarif FILE] fs
  python3 -I tools/ci/trivy_scan.py --trivy NAME:TAG@sha256:DIGEST [--sarif FILE] image --image REF
"""

import argparse
import io
import json
import pathlib
import re
import subprocess
import sys
import tarfile
import tempfile

IMAGE_RE = re.compile(r"^[a-z0-9./-]+:[A-Za-z0-9._-]+@sha256:[0-9a-f]{64}$")
PLANTED_IMAGE = ("mirror.gcr.io/library/alpine:3.12.0"
                 "@sha256:a15790640a6690aa1730c38cf0a440e2aa44aaca9b0e8931a9f2b0d7cc90fd65")
CACHE_VOLUME = "diallak-trivy-cache"
TIMEOUT_SECONDS = 900
MAX_FILES = 4096
MAX_REPORT_BYTES = 64 * 1024 * 1024
SCAN_FLAGS = ["--scanners", "vuln,secret,misconfig", "--severity", "HIGH,CRITICAL",
              "--ignore-unfixed", "--ignorefile", "", "--skip-version-check",
              "--disable-telemetry", "--format", "json", "--exit-code", "1"]
# Unpacks the tar on stdin and runs trivy on it; arguments come in as "$@".
UNPACK_AND_SCAN = "mkdir -p /scan && tar -xf - -C /scan && cd /scan && exec trivy \"$@\""


class ScanError(Exception):
    """A scan that cannot be trusted or that found a violation."""


def run(command, stdin=b""):
    try:
        return subprocess.run(command, input=stdin, capture_output=True,
                              timeout=TIMEOUT_SECONDS, check=False)
    except (OSError, subprocess.TimeoutExpired) as err:
        raise ScanError(f"{command[0]} did not complete: {err}") from err


def report_of(result):
    """Trivy exits 1 exactly when it reports a finding; anything else is an error."""
    stderr = result.stderr.decode("utf-8", "replace")[-4000:]
    if result.returncode not in (0, 1) or len(result.stdout) > MAX_REPORT_BYTES:
        sys.stderr.write(stderr)
        raise ScanError(f"trivy exited {result.returncode}")
    try:
        report = json.loads(result.stdout)
    except json.JSONDecodeError as err:
        sys.stderr.write(stderr)
        raise ScanError(f"no JSON report (exit {result.returncode}): {err}") from err
    if (result.returncode == 1) != bool(findings(report)):
        sys.stderr.write(stderr)
        raise ScanError(f"trivy exited {result.returncode} with {len(findings(report))} finding(s)")
    return report


def findings(report):
    found = []
    for result in report.get("Results") or []:
        for key in ("Vulnerabilities", "Misconfigurations", "Secrets"):
            for item in result.get(key) or []:
                ident = item.get("VulnerabilityID") or item.get("ID") or item.get("RuleID")
                found.append((result.get("Target"), ident, item.get("Severity"), item.get("Title", "")))
    return found


def trivy_fs(docker, trivy, tar_bytes, sarif=None):
    command = [docker, "run", "--rm", "-i", "--volume", f"{CACHE_VOLUME}:/var/cache/trivy",
               "--entrypoint", "sh", trivy, "-ec", UNPACK_AND_SCAN, "trivy",
               "--cache-dir", "/var/cache/trivy", "fs", *SCAN_FLAGS, "."]
    return sarif_of(docker, trivy, run(command, tar_bytes), sarif)


def trivy_image(docker, trivy, image_tar, sarif=None):
    command = [docker, "run", "--rm", "--volume", f"{CACHE_VOLUME}:/var/cache/trivy",
               "--volume", f"{image_tar}:/in/image.tar:ro", trivy,
               "--cache-dir", "/var/cache/trivy", "image", *SCAN_FLAGS, "--input", "/in/image.tar"]
    return sarif_of(docker, trivy, run(command), sarif)


def sarif_of(docker, trivy, result, sarif):
    """Checks the report and, with a SARIF path, converts it there before any finding fails."""
    report = report_of(result)
    if sarif is None:
        return report
    sarif.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory() as temp:
        (pathlib.Path(temp) / "report.json").write_bytes(result.stdout)
        converted = run([docker, "run", "--rm", "--network", "none",
                         "--volume", f"{temp}:/in:ro", "--volume", f"{sarif.parent}:/out", trivy,
                         "convert", "--format", "sarif", "--output", f"/out/{sarif.name}",
                         "/in/report.json"])
    if converted.returncode != 0 or not sarif.is_file():
        sys.stderr.write(converted.stderr.decode("utf-8", "replace")[-4000:])
        raise ScanError(f"trivy convert wrote no SARIF report (exit {converted.returncode})")
    return report


def tar_of(root, names):
    buffer = io.BytesIO()
    with tarfile.open(fileobj=buffer, mode="w") as archive:
        for name in names:
            archive.add(root / name, arcname=name, recursive=False)
    return buffer.getvalue()


def by_type(report, kind):
    return {r.get("Target"): r for r in report.get("Results") or [] if r.get("Type") == kind}


def planted_tree(directory):
    # The key pair is joined from pieces here, so that neither the key nor a
    # keyword next to a long literal is in the source for gitleaks to report.
    key_id = "".join(("AKIA", "Z7Q3XWERT5YUIOPA"))
    key_value = "/".join(("wJalrXUtnFEMI", "K7MDENG", "bPxRfiCYZXAMPLEKEYQ"))
    files = {
        "go.mod": "module example.com/planted\n\ngo 1.22\n\nrequire golang.org/x/text v0.3.0\n",
        "Dockerfile": "FROM debian:13\nRUN apt-get update\n",
        "config.env": f"aws_access_key_id = {key_id}\naws_secret_access_key = {key_value}\n",
    }
    for name, text in files.items():
        (directory / name).write_text(text, encoding="utf-8")
    return sorted(files)


def self_test_fs(docker, trivy):
    with tempfile.TemporaryDirectory() as temp:
        directory = pathlib.Path(temp)
        report = trivy_fs(docker, trivy, tar_of(directory, planted_tree(directory)))
    reported = {target for target, _, _, _ in findings(report)}
    missing = sorted({"go.mod", "Dockerfile", "config.env"} - reported)
    if missing:
        raise ScanError(f"planted case(s) not reported: {', '.join(missing)}")
    print(f"Trivy fs self-test: planted module, Dockerfile and key reported "
          f"({len(findings(report))} findings)")


def tracked(root):
    listed = run(["git", "-C", str(root), "ls-files", "-z"])
    if listed.returncode != 0:
        raise ScanError(f"git ls-files failed: {listed.stderr.decode('utf-8', 'replace').strip()}")
    names = [n for n in listed.stdout.decode("utf-8").split("\0") if n and (root / n).is_file()]
    if not names or len(names) > MAX_FILES:
        raise ScanError(f"{len(names)} tracked files, expected 1 to {MAX_FILES}")
    return names


def is_dockerfile(name):
    base = pathlib.PurePosixPath(name).name
    return base == "Dockerfile" or base.startswith("Dockerfile.") or base.endswith(".Dockerfile")


def fail_on(found):
    for target, ident, severity, title in found:
        print(f"{target}: {ident} ({severity}): {title}", file=sys.stderr)
    if found:
        raise ScanError(f"{len(found)} finding(s)")


def scan_fs(docker, trivy, root, sarif):
    names = tracked(root)
    report = trivy_fs(docker, trivy, tar_of(root, names), sarif)
    fail_on(findings(report))
    modules = {n for n in names if pathlib.PurePosixPath(n).name == "go.mod"}
    dockerfiles = {n for n in names if is_dockerfile(n)}
    missing = sorted((modules - set(by_type(report, "gomod")))
                     | (dockerfiles - set(by_type(report, "dockerfile"))))
    if missing or not modules:
        raise ScanError(f"report left out: {missing or 'every go.mod'}")
    print(f"Trivy fs: {len(names)} tracked files, {len(modules)} go.mod, "
          f"{len(dockerfiles)} Dockerfiles, 0 findings")


def saved(docker, image, directory):
    path = directory / "image.tar"
    result = run([docker, "save", "--output", str(path), image])
    if result.returncode != 0:
        raise ScanError(f"docker save {image} failed: {result.stderr.decode('utf-8', 'replace').strip()}")
    return path


def self_test_image(docker, trivy):
    pulled = run([docker, "pull", "--quiet", PLANTED_IMAGE])
    if pulled.returncode != 0:
        raise ScanError(f"docker pull {PLANTED_IMAGE} failed")
    with tempfile.TemporaryDirectory() as temp:
        report = trivy_image(docker, trivy, saved(docker, PLANTED_IMAGE, pathlib.Path(temp)))
    found = [f for f in findings(report) if f[1] and f[1].startswith("CVE-")]
    if not found or not by_type(report, "alpine"):
        raise ScanError(f"planted image {PLANTED_IMAGE} reported no vulnerable OS package")
    print(f"Trivy image self-test: planted image reported ({len(found)} vulnerabilities)")


def scan_image(docker, trivy, image, binary, sarif):
    with tempfile.TemporaryDirectory() as temp:
        report = trivy_image(docker, trivy, saved(docker, image, pathlib.Path(temp)), sarif)
    fail_on(findings(report))
    os_results = [r for r in report.get("Results") or [] if r.get("Class") == "os-pkgs"]
    binaries = by_type(report, "gobinary")
    if not os_results or binary not in binaries or not binaries[binary].get("Packages"):
        raise ScanError(f"report left out the OS packages or the binary '{binary}' "
                        f"(Go binaries: {sorted(binaries)})")
    print(f"Trivy image: {image}: OS packages and binary '{binary}' "
          f"({len(binaries[binary]['Packages'])} modules) scanned, 0 findings")


def parse_args():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--trivy", required=True, help="Trivy image pinned by digest")
    parser.add_argument("--docker", default="docker")
    parser.add_argument("--root", default=".")
    parser.add_argument("--sarif", type=pathlib.Path, help="also write the real scan's findings as SARIF here")
    modes = parser.add_subparsers(dest="mode", required=True)
    modes.add_parser("fs", help="scan the tracked files")
    image = modes.add_parser("image", help="scan an image from the local Docker daemon")
    image.add_argument("--image", required=True, help="local image reference")
    image.add_argument("--binary", default="app", help="path of the Go binary inside the image")
    return parser.parse_args()


def main():
    args = parse_args()
    sarif = args.sarif.resolve() if args.sarif else None
    try:
        if not IMAGE_RE.match(args.trivy):
            raise ScanError(f"image '{args.trivy}' is not pinned as NAME:TAG@sha256:DIGEST")
        if args.mode == "fs":
            self_test_fs(args.docker, args.trivy)
            scan_fs(args.docker, args.trivy, pathlib.Path(args.root).resolve(), sarif)
        else:
            self_test_image(args.docker, args.trivy)
            scan_image(args.docker, args.trivy, args.image, args.binary, sarif)
    except ScanError as err:
        print(f"Trivy scan failed: {err}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
