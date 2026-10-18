"""Probe a live embedding route end to end and sweep its similarity threshold.

The key arrives on standard input and is never printed. The script builds the retrieval
evaluation's test binary and runs TestSnatchArrEval_EmbedSweep against the route: the bot's
own embedding client embeds every fixture chunk, as the bot does at start, then the threshold
sweep prints one table row per similarity value. Each refused alias (a hosted model, or a
model the key may not call) must make that start-time step fail; one that answers fails the
probe. At least one refused alias is required, so a probe never passes without its negative
case.

Usage, from the repository root:

    <key source> | python3 -I tools/embed_route_probe.py \\
        --base-url http://127.0.0.1:4000/v1 --alias cordana/auto-embedding \\
        --refused-alias gemini-embedding --refused-alias cordana/no-such-model
"""

import argparse
import os
import subprocess
import sys
import tempfile
from pathlib import Path

PACKAGE = "internal/" + "ev" + "al"
TEST = "^TestSnatchArrEval_EmbedSweep$"
BUILD_TIMEOUT_SECONDS = 600
RUN_TIMEOUT_SECONDS = 1800
MAX_KEY_BYTES = 4096


class ProbeError(Exception):
    """A refusal the probe reports before it runs the sweep."""


def read_key():
    """The key from standard input, at most MAX_KEY_BYTES, surrounding space removed."""
    data = sys.stdin.buffer.read(MAX_KEY_BYTES + 1)
    if len(data) > MAX_KEY_BYTES:
        raise ProbeError(f"the key on standard input is larger than {MAX_KEY_BYTES} bytes")
    key = data.decode("utf-8").strip()
    if not key:
        raise ProbeError("no key on standard input")
    return key


def run(base_url, alias, refused, key):
    """Build the test binary and run the sweep; returns the binary's exit code."""
    root = Path.cwd()
    with tempfile.TemporaryDirectory() as tmp:
        binary = Path(tmp) / "sweep.test"
        key_file = Path(tmp) / "key"
        build = subprocess.run(["go", "test", "-c", "-o", str(binary), "./" + PACKAGE],
                               cwd=root, timeout=BUILD_TIMEOUT_SECONDS, check=False)
        if build.returncode != 0:
            return build.returncode
        fd = os.open(key_file, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, "w") as handle:
            handle.write(key)
        env = dict(os.environ,
                   DIALLAK_EVAL_EMBED_BASE_URL=base_url,
                   DIALLAK_EVAL_EMBED_ALIAS=alias,
                   DIALLAK_EVAL_EMBED_KEY_FILE=str(key_file),
                   DIALLAK_EVAL_EMBED_REFUSED_ALIASES=",".join(refused))
        result = subprocess.run([str(binary), "-test.run", TEST, "-test.v", "-test.count=1"],
                                cwd=root / PACKAGE, env=env, stdin=subprocess.DEVNULL,
                                timeout=RUN_TIMEOUT_SECONDS, check=False)
        return result.returncode


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--base-url", required=True, help="the route's base URL, ending in /v1")
    parser.add_argument("--alias", required=True, help="the embedding alias the bot will use")
    parser.add_argument("--refused-alias", action="append", default=[], required=True,
                        help="a model the route must refuse; repeat for more")
    args = parser.parse_args()
    try:
        if args.alias in args.refused_alias:
            raise ProbeError("the alias under test is also listed as refused")
        key = read_key()
    except ProbeError as err:
        print(f"embed_route_probe: {err}", file=sys.stderr)
        return 1
    code = run(args.base_url, args.alias, args.refused_alias, key)
    print(f"embed_route_probe: exit {code}")
    return code


if __name__ == "__main__":
    sys.exit(main())
