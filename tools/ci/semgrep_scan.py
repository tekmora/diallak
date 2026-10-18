"""Run the repository's Semgrep rules in a digest-pinned image, fixtures first.

Each rule in .semgrep.yml needs a fixture in tools/ci/semgrep/testdata/ with at
least one `// ruleid:` line it must report and one `// ok:` line it must not.
Every fixture starts with the `//go:build semgrep_fixture` constraint, so no Go
build, vet or gosec run ever loads the code the rules are meant to report.
Semgrep's test mode replays every fixture; only when all pass is the scan of
the tracked Go files trusted. The scan fails on any finding, on any Semgrep
error, and when the report does not list exactly the files that were passed.
With --sarif, the scan also writes its findings as SARIF to that file, for
upload to code scanning; the file is written before the scan fails.

Usage: python3 -I tools/ci/semgrep_scan.py --image NAME:TAG@sha256:DIGEST [--sarif FILE]
"""

import argparse
import json
import pathlib
import re
import subprocess
import sys

RULES = ".semgrep.yml"
FIXTURES = "tools/ci/semgrep/testdata"
FIXTURE_CONSTRAINT = "//go:build semgrep_fixture\n"
MAX_FIXTURES = 64
MAX_TARGETS = 4096
TIMEOUT_SECONDS = 600
IMAGE_RE = re.compile(r"^[a-z0-9./-]+:[A-Za-z0-9._-]+@sha256:[0-9a-f]{64}$")
RULE_ID_RE = re.compile(r"^  - id: ([a-z0-9-]+)$", re.MULTILINE)
ANNOTATION_RE = re.compile(r"//\s*(ruleid|ok):\s*([a-z0-9-]+)\s*$", re.MULTILINE)
SEMGREP_FLAGS = ["--metrics=off", "--disable-version-check", "--oss-only", "--jobs", "1"]


class ScanError(Exception):
    """A scan that cannot be trusted or that found a violation."""


def run(command, root):
    try:
        return subprocess.run(command, cwd=root, capture_output=True, text=True,
                              timeout=TIMEOUT_SECONDS, check=False)
    except (OSError, subprocess.TimeoutExpired) as err:
        raise ScanError(f"{command[0]} did not complete: {err}") from err


def semgrep(docker, image, root, args, out_dir=None):
    volumes = ["--volume", f"{root}:/src:ro"]
    if out_dir is not None:
        volumes += ["--volume", f"{out_dir}:/out"]
    command = [docker, "run", "--rm", "--network", "none", *volumes, "--workdir", "/src",
               "--entrypoint", "semgrep", image, *args]
    return run(command, root)


def sarif_args(sarif):
    """The output flag and the directory to mount for a SARIF copy of the report."""
    if sarif is None:
        return [], None
    sarif.parent.mkdir(parents=True, exist_ok=True)
    return [f"--sarif-output=/out/{sarif.name}"], sarif.parent


def rule_ids(root):
    ids = RULE_ID_RE.findall((root / RULES).read_text(encoding="utf-8"))
    if not ids:
        raise ScanError(f"{RULES} declares no rule")
    if len(ids) != len(set(ids)):
        raise ScanError(f"{RULES} declares a rule id twice")
    return set(ids)


def fixtures(root, ids):
    files = sorted((root / FIXTURES).glob("*.go"))
    if not files or len(files) > MAX_FIXTURES:
        raise ScanError(f"{FIXTURES}: {len(files)} fixtures, expected 1 to {MAX_FIXTURES}")
    seen = {"ruleid": set(), "ok": set()}
    for path in files:
        text = path.read_text(encoding="utf-8")
        if not text.startswith(FIXTURE_CONSTRAINT):
            raise ScanError(f"{path.name}: does not start with {FIXTURE_CONSTRAINT.strip()!r}")
        for kind, rule in ANNOTATION_RE.findall(text):
            if rule not in ids:
                raise ScanError(f"{path.name}: annotation names unknown rule '{rule}'")
            seen[kind].add(rule)
    for kind, covered in sorted(seen.items()):
        missing = sorted(ids - covered)
        if missing:
            raise ScanError(f"no `{kind}:` fixture line for rule(s): {', '.join(missing)}")
    return [path.relative_to(root).as_posix() for path in files]


def self_test(docker, image, root, ids):
    names = fixtures(root, ids)
    for name in names:
        result = semgrep(docker, image, root, ["scan", "--test", "--config", RULES, name, *SEMGREP_FLAGS])
        if result.returncode != 0 or "All tests passed" not in result.stdout:
            sys.stderr.write(result.stdout[-4000:] + result.stderr[-4000:])
            raise ScanError(f"fixture {name} did not pass (exit {result.returncode})")
    print(f"Semgrep self-test: {len(names)} fixtures passed, {len(ids)} rules covered both ways")


def targets(root):
    listed = run(["git", "ls-files", "-z", "--", "*.go"], root)
    if listed.returncode != 0:
        raise ScanError(f"git ls-files failed: {listed.stderr.strip()}")
    names = [n for n in listed.stdout.split("\0") if n and not n.startswith(FIXTURES + "/")]
    if not names or len(names) > MAX_TARGETS:
        raise ScanError(f"{len(names)} Go files to scan, expected 1 to {MAX_TARGETS}")
    return names


def scan(docker, image, root, sarif):
    names = targets(root)
    output, out_dir = sarif_args(sarif)
    result = semgrep(docker, image, root,
                     ["scan", "--config", RULES, "--error", "--strict", "--json", *output,
                      *SEMGREP_FLAGS, "--", *names], out_dir)
    if sarif is not None and not sarif.is_file():
        raise ScanError(f"semgrep wrote no SARIF report to {sarif}")
    try:
        report = json.loads(result.stdout)
    except json.JSONDecodeError as err:
        sys.stderr.write(result.stderr[-4000:])
        raise ScanError(f"no JSON report (exit {result.returncode}): {err}") from err
    for finding in report.get("results", []):
        print(f"{finding['path']}:{finding['start']['line']}: {finding['check_id']}: "
              f"{finding['extra']['message']}", file=sys.stderr)
    for error in report.get("errors", []):
        print(f"semgrep error: {error.get('type')}: {error.get('message')}", file=sys.stderr)
    scanned = set(report.get("paths", {}).get("scanned", []))
    if report.get("results") or report.get("errors"):
        raise ScanError(f"{len(report.get('results', []))} finding(s), {len(report.get('errors', []))} error(s)")
    if scanned != set(names):
        skipped = sorted(set(names) - scanned)[:10]
        raise ScanError(f"report scanned {len(scanned)} of {len(names)} files; not scanned: {skipped}")
    if result.returncode != 0:
        raise ScanError(f"semgrep exited {result.returncode} with a clean report")
    print(f"Semgrep: {len(names)} files scanned, 0 findings")


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--image", required=True, help="Semgrep image pinned by digest")
    parser.add_argument("--docker", default="docker")
    parser.add_argument("--root", default=".")
    parser.add_argument("--sarif", type=pathlib.Path, help="also write the findings as SARIF here")
    args = parser.parse_args()
    root = pathlib.Path(args.root).resolve()
    sarif = args.sarif.resolve() if args.sarif else None
    try:
        if not IMAGE_RE.match(args.image):
            raise ScanError(f"image '{args.image}' is not pinned as NAME:TAG@sha256:DIGEST")
        ids = rule_ids(root)
        self_test(args.docker, args.image, root, ids)
        scan(args.docker, args.image, root, sarif)
    except ScanError as err:
        print(f"Semgrep scan failed: {err}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
