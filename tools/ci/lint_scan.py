"""Run golangci-lint with the repository's .golangci.yml and fail on any finding.

The self-test first lints a module planted at run time: a clean file, which must give no
finding, and planted defects for linters and the gofmt formatter the configuration enables,
which must each be reported by that linter or formatter. The clean file holds one function at
each HISS-04 limit (cyclomatic 12, cognitive 15, 55 statements, 60 lines), and each planted
complexity defect is one step over its limit, so both sides of every limit are checked. Only
then is the result on the tracked code trusted.
"""

import argparse
import json
import subprocess
import sys
import tempfile
from pathlib import Path

CONFIG = ".golangci.yml"
ISSUES_EXIT = 3
MAX_REPORT_BYTES = 16 * 1024 * 1024
LINT_TIMEOUT_SECONDS = 900



def branches(name, count):
    """A function of count sequential ifs: cyclomatic complexity count + 1, cognitive count."""
    body = "".join(f"\tif x == {i} {{\n\t\tx++\n\t}}\n" for i in range(count))
    return f"// {name} counts.\nfunc {name}(x int) int {{\n{body}\treturn x\n}}\n"


def nested(name, depth):
    """A function of depth nested ifs: cognitive complexity 1 + 2 + ... + depth."""
    opening = "".join("\t" * (i + 1) + f"if x > {i} {{\n" for i in range(depth))
    closing = "".join("\t" * (i + 1) + "}\n" for i in reversed(range(depth)))
    return f"// {name} counts.\nfunc {name}(x int) int {{\n{opening}" + "\t" * (depth + 1) + f"x++\n{closing}\treturn x\n}}\n"


def statements(name, count):
    """A function of count statements, one per line."""
    body = "\tx++\n" * (count - 1)
    return f"// {name} counts.\nfunc {name}(x int) int {{\n{body}\treturn x\n}}\n"


def lines(name, count):
    """A function of count lines between its braces and two statements."""
    items = "".join(f"\t\t{i},\n" for i in range(count - 3))
    return f"// {name} counts.\nfunc {name}(x int) int {{\n\tv := []int{{\n{items}\t}}\n\treturn x + len(v)\n}}\n"


CLEAN = "\n".join([
    "package planted\n",
    "// Sum adds two numbers.\nfunc Sum(a, b int) int {\n\treturn a + b\n}\n",
    branches("AtCyclomaticLimit", 11),
    nested("AtCognitiveLimit", 5),
    statements("AtStatementLimit", 55),
    lines("AtLineLimit", 60),
])

# One file per linter, so that one finding cannot stand in for another.
PLANTED = {
    "errcheck": """package planted

import "os"

// Remove drops a file and ignores the error.
func Remove(name string) {
	os.Remove(name)
}
""",
    "ineffassign": """package planted

// Twice assigns a value that is never read.
func Twice() int {
	n := 1
	n = 2
	n = 3
	return n
}
""",
    # gofmt runs as a formatter in .golangci.yml; it reports a file that
    # gofmt would rewrite, here an indent of spaces and a binary expression
    # without spaces.
    "gofmt": """package planted

// Add adds two numbers.
func Add(a, b int) int {
  return a+b
}
""",
    "govet": """package planted

import "fmt"

// Show formats a number with a verb for strings.
func Show(n int) string {
	return fmt.Sprintf("%s", n)
}
""",
    # One step over each HISS-04 limit. A key after an underscore names a
    # second defect for the same linter.
    "gocyclo": "package planted\n\n" + branches("OverCyclomaticLimit", 12),
    "gocognit": "package planted\n\n" + nested("OverCognitiveLimit", 6),
    "funlen": "package planted\n\n" + statements("OverStatementLimit", 56),
    "funlen_lines": "package planted\n\n" + lines("OverLineLimit", 61),
}


class LintError(ValueError):
    """golangci-lint did not complete, or its report cannot be trusted."""


def findings(golangci_lint, directory, config):
    """Return (linter, file, line) for every issue golangci-lint reports in directory."""
    with tempfile.TemporaryDirectory() as scratch:
        report = Path(scratch) / "report.json"
        command = [
            str(Path(golangci_lint).resolve()), "run", "--config", str(Path(config).resolve()),
            "--issues-exit-code", str(ISSUES_EXIT), "--max-issues-per-linter", "0", "--max-same-issues", "0",
            "--output.json.path", str(report), "--output.text.path", "stderr", "./...",
        ]
        result = subprocess.run(command, cwd=directory, capture_output=True, text=True, timeout=LINT_TIMEOUT_SECONDS, check=False)
        if result.returncode not in (0, ISSUES_EXIT):
            raise LintError(f"golangci-lint did not complete (exit {result.returncode}): {result.stderr.strip()[-600:]}")
        if not report.is_file() or report.stat().st_size > MAX_REPORT_BYTES:
            raise LintError("golangci-lint wrote no usable report")
        issues = json.loads(report.read_text(encoding="utf-8")).get("Issues") or []
    found = [(item["FromLinter"], item["Pos"]["Filename"], item["Pos"]["Line"]) for item in issues]
    if bool(found) != (result.returncode == ISSUES_EXIT):
        raise LintError("the golangci-lint exit status contradicts its report")
    return found


def planted_module(root, files):
    module = Path(root)
    (module / "go.mod").write_text("module example.invalid/planted\n\ngo 1.27\n", encoding="utf-8")
    for name, text in files.items():
        (module / f"{name}.go").write_text(text, encoding="utf-8")
    return module


def self_test(golangci_lint, config):
    """Prove the gate against planted defects before its result on the tracked code is trusted."""
    with tempfile.TemporaryDirectory() as root:
        clean = findings(golangci_lint, planted_module(root, {"clean": CLEAN}), config)
    if clean:
        raise LintError(f"self-test 'clean': expected no finding, got {clean}")
    for planted, text in PLANTED.items():
        linter = planted.partition("_")[0]
        with tempfile.TemporaryDirectory() as root:
            found = findings(golangci_lint, planted_module(root, {"clean": CLEAN, planted: text}), config)
        if not any(name == linter and Path(file).name == f"{planted}.go" for name, file, _ in found):
            raise LintError(f"self-test '{planted}': the planted defect was not reported by {linter}; got {found}")
    print(f"Lint self-test: the clean module and {len(PLANTED)} planted defects behaved as required")


def main(arguments=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--golangci-lint", type=Path, required=True)
    parser.add_argument("--repository", type=Path, default=Path("."))
    parser.add_argument("--self-test", action="store_true")
    options = parser.parse_args(arguments)
    config = options.repository / CONFIG
    try:
        if not config.is_file():
            raise LintError(f"{CONFIG} is missing")
        if options.self_test:
            self_test(options.golangci_lint, config)
            return 0
        found = findings(options.golangci_lint, options.repository, config)
    except (LintError, OSError, subprocess.SubprocessError, json.JSONDecodeError, KeyError, TypeError) as error:
        print(f"Lint failed: {error}", file=sys.stderr)
        return 2
    for linter, file, line in found:
        print(f"{file}:{line}: {linter}")
    print(f"Lint: {len(found)} finding(s)")
    return 1 if found else 0


if __name__ == "__main__":
    sys.exit(main())
