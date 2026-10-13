"""Show that govulncheck and gosec fail the vulncheck job on a planted finding.

The job trusts the exit status of both scanners on the tracked code. This script first
runs each scanner on a module planted at run time: a clean module, which must pass, and a
module with a known defect, which must fail with that defect named. Only then are the
scanners' results on the tracked code trusted.

- govulncheck: golang.org/x/text v0.3.7 with a call to language.ParseAcceptLanguage, the
  symbol of GO-2022-1059. govulncheck exits 3 only for a vulnerability the code calls.
- gosec, with the repository's configuration: a path read from a variable (G304) and an
  MD5 hash (G401, G501).
"""

import argparse
import json
import subprocess
import sys
import tempfile
from pathlib import Path

SCAN_TIMEOUT_SECONDS = 600
MAX_REPORT_BYTES = 16 * 1024 * 1024
GOVULNCHECK_FINDING_EXIT = 3
GOSEC_FINDING_EXIT = 1

VULNERABILITY = "GO-2022-1059"
VULNERABLE_MODULE = """module example.invalid/planted

go 1.27

require golang.org/x/text v0.3.7
"""
VULNERABLE_CALL = """package main

import (
	"fmt"

	"golang.org/x/text/language"
)

func main() {
	tags, _, err := language.ParseAcceptLanguage("en-US")
	fmt.Println(tags, err)
}
"""

PLAIN_MODULE = """module example.invalid/planted

go 1.27
"""
CLEAN = """package planted

// Sum adds two numbers.
func Sum(a, b int) int {
	return a + b
}
"""
INSECURE = """package planted

import (
	"crypto/md5"
	"os"
)

// Read opens a path the caller chose.
func Read(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// Hash uses a broken hash.
func Hash(b []byte) [16]byte {
	return md5.Sum(b)
}
"""
GOSEC_RULES = {"G304", "G401", "G501"}


class SelfTestError(ValueError):
    """A scanner did not behave as the planted case requires."""


def run(command, directory):
    return subprocess.run(command, cwd=directory, capture_output=True, text=True, timeout=SCAN_TIMEOUT_SECONDS, check=False)


def write_module(root, files):
    module = Path(root)
    for name, text in files.items():
        (module / name).write_text(text, encoding="utf-8")
    return module


def govulncheck_case(govulncheck, files):
    """Return the exit status and output of govulncheck on a module planted from files."""
    with tempfile.TemporaryDirectory() as root:
        module = write_module(root, files)
        tidy = run(["go", "mod", "tidy"], module)
        if tidy.returncode != 0:
            raise SelfTestError(f"go mod tidy failed on the planted module: {tidy.stderr.strip()[-600:]}")
        result = run([str(Path(govulncheck).resolve()), "./..."], module)
    return result.returncode, result.stdout + result.stderr


def check_govulncheck(govulncheck):
    status, output = govulncheck_case(govulncheck, {"go.mod": PLAIN_MODULE, "clean.go": CLEAN})
    if status != 0:
        raise SelfTestError(f"govulncheck on the clean module: expected exit 0, got {status}: {output.strip()[-600:]}")
    status, output = govulncheck_case(govulncheck, {"go.mod": VULNERABLE_MODULE, "main.go": VULNERABLE_CALL})
    if status != GOVULNCHECK_FINDING_EXIT or f": {VULNERABILITY}" not in output:
        raise SelfTestError(f"govulncheck on the planted call: expected exit 3 naming {VULNERABILITY}, got {status}: {output.strip()[-600:]}")


def gosec_case(gosec, config, source):
    """Return the exit status and the rule ids gosec reports on a module planted from source."""
    with tempfile.TemporaryDirectory() as root:
        module = write_module(root, {"go.mod": PLAIN_MODULE, "planted.go": source})
        report = module / "report.json"
        result = run([str(Path(gosec).resolve()), "-conf", str(Path(config).resolve()), "-fmt", "json", "-out", str(report), "./..."], module)
        if not report.is_file() or report.stat().st_size > MAX_REPORT_BYTES:
            raise SelfTestError(f"gosec wrote no usable report (exit {result.returncode}): {result.stderr.strip()[-600:]}")
        issues = json.loads(report.read_text(encoding="utf-8")).get("Issues") or []
    return result.returncode, {issue["rule_id"] for issue in issues}


def check_gosec(gosec, config):
    status, rules = gosec_case(gosec, config, CLEAN)
    if status != 0 or rules:
        raise SelfTestError(f"gosec on the clean module: expected exit 0 and no finding, got {status} {sorted(rules)}")
    status, rules = gosec_case(gosec, config, INSECURE)
    if status != GOSEC_FINDING_EXIT or not GOSEC_RULES <= rules:
        raise SelfTestError(f"gosec on the planted code: expected exit 1 with {sorted(GOSEC_RULES)}, got {status} {sorted(rules)}")


def main(arguments=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--govulncheck", type=Path, required=True)
    parser.add_argument("--gosec", type=Path, required=True)
    parser.add_argument("--gosec-config", type=Path, default=Path(".gosec.json"))
    options = parser.parse_args(arguments)
    try:
        check_govulncheck(options.govulncheck)
        check_gosec(options.gosec, options.gosec_config)
    except (SelfTestError, OSError, subprocess.SubprocessError, json.JSONDecodeError, KeyError, TypeError) as error:
        print(f"Scanner self-test failed: {error}", file=sys.stderr)
        return 2
    print(f"Scanner self-test: govulncheck reported {VULNERABILITY}, gosec reported {', '.join(sorted(GOSEC_RULES))}; both clean modules passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
