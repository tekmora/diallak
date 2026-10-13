"""Run the built image, check its user, architecture and size, planted cases first.

The image is checked without a network and without configuration, so nothing
reaches Discord or a model:

- run: started with no environment, the binary exits 1 and reports
  "diallak: load configuration:"; started with -server=bogus, it exits 1 and
  reports the invalid mode. Either proves the binary starts and runs its own
  code on this machine.
- user: the image runs as 65532:65532.
- arch: /app is an ELF binary for the expected architecture.
- size: the files in the image add up to at most --max-image-bytes, and /app
  is at most --max-binary-bytes. Both are read from one `docker export` of a
  created container, so the figure does not depend on the image store.

Planted cases run first and each must fail its own check: the limits one byte
under the measured sizes, an architecture the image is not built for, and two
images derived from the real one, one whose entrypoint does not exist and one
that runs as root.

Usage:
  python3 -I tools/ci/image_smoke.py --image REF --arch amd64 \
      --max-image-bytes N --max-binary-bytes N
"""

import argparse
import io
import subprocess
import sys
import tarfile

TIMEOUT_SECONDS = 300
MAX_MEMBERS = 4096
USER = "65532:65532"
# ELF e_machine values (System V ABI): EM_X86_64 and EM_AARCH64.
ELF_MACHINE = {"amd64": 62, "arm64": 183}
RUNS = (
    ([], "diallak: load configuration:"),
    (["-server=bogus"], 'diallak: invalid -server mode "bogus"'),
)


class SmokeError(Exception):
    """A check that cannot be trusted or that found a violation."""


def run(command, stdin=None):
    try:
        return subprocess.run(command, input=stdin, capture_output=True,
                              timeout=TIMEOUT_SECONDS, check=False)
    except (OSError, subprocess.TimeoutExpired) as err:
        raise SmokeError(f"{command[0]} did not complete: {err}") from err


def checked(command, stdin=None):
    result = run(command, stdin)
    if result.returncode != 0:
        stderr = result.stderr.decode("utf-8", "replace").strip()[-2000:]
        raise SmokeError(f"{' '.join(command[:3])} exited {result.returncode}: {stderr}")
    return result.stdout


def measure(docker, image):
    """Return (sum of regular file sizes, size of /app, first 20 bytes of /app)."""
    container = checked([docker, "create", image]).decode().strip()
    try:
        exported = checked([docker, "export", container])
    finally:
        run([docker, "rm", "--force", container])
    total, app_size, app_head = 0, None, b""
    with tarfile.open(fileobj=io.BytesIO(exported), mode="r:") as archive:
        for count, member in enumerate(archive):
            if count >= MAX_MEMBERS:
                raise SmokeError(f"image has more than {MAX_MEMBERS} entries")
            if not member.isreg():
                continue
            total += member.size
            if member.name.lstrip("./") == "app":
                app_size = member.size
                app_head = archive.extractfile(member).read(20)
    if app_size is None:
        raise SmokeError("image has no /app")
    return total, app_size, app_head


def check_run(docker, image, arch):
    failures = []
    for args, want in RUNS:
        result = run([docker, "run", "--rm", "--platform", f"linux/{arch}", "--network", "none", "--read-only", image, *args])
        stderr = result.stderr.decode("utf-8", "replace")
        if result.returncode != 1 or not stderr.startswith(want):
            failures.append(f"run {args or 'without configuration'}: exit {result.returncode}, "
                            f"want 1 and stderr starting {want!r}, got {stderr.strip()[:200]!r}")
    return failures


def check_user(docker, image):
    user = checked([docker, "image", "inspect", "--format", "{{.Config.User}}", image]).decode().strip()
    return [] if user == USER else [f"user: {user or 'unset'}, want {USER}"]


def check_arch(head, arch):
    if arch not in ELF_MACHINE:
        return [f"arch: {arch} is not one of {sorted(ELF_MACHINE)}"]
    if len(head) < 20 or head[:4] != b"\x7fELF":
        return ["arch: /app is not an ELF binary"]
    machine = int.from_bytes(head[18:20], "little")
    return [] if machine == ELF_MACHINE[arch] else [f"arch: /app has ELF machine {machine}, want {ELF_MACHINE[arch]} ({arch})"]


def check_size(total, app_size, max_image, max_binary):
    failures = []
    if total > max_image:
        failures.append(f"size: image files {total:,} bytes, limit {max_image:,}")
    if app_size > max_binary:
        failures.append(f"size: /app {app_size:,} bytes, limit {max_binary:,}")
    return failures


def derived(docker, image, arch, tag, line):
    dockerfile = f"FROM {image}\n{line}\n".encode()
    checked([docker, "build", "--quiet", "--platform", f"linux/{arch}", "--tag", tag, "-"], dockerfile)
    return tag


def expect_failure(name, failures, prefix):
    if not any(f.startswith(prefix) for f in failures):
        raise SmokeError(f"planted case '{name}' was not reported by the {prefix.rstrip(':')} check")
    print(f"planted case '{name}': reported ({failures[0]})")


def self_test(docker, image, arch, total, app_size, head):
    expect_failure("image one byte over", check_size(total, app_size, total - 1, app_size), "size:")
    expect_failure("binary one byte over", check_size(total, app_size, total, app_size - 1), "size:")
    other = next(a for a in sorted(ELF_MACHINE) if a != arch)
    expect_failure(f"built for {arch}, expected {other}", check_arch(head, other), "arch:")
    planted = []
    try:
        planted.append(derived(docker, image, arch, "diallak-smoke-planted:missing", 'ENTRYPOINT ["/missing"]'))
        expect_failure("entrypoint missing", check_run(docker, planted[-1], arch), "run")
        planted.append(derived(docker, image, arch, "diallak-smoke-planted:root", "USER 0:0"))
        expect_failure("runs as root", check_user(docker, planted[-1]), "user:")
    finally:
        for tag in planted:
            run([docker, "image", "rm", "--force", tag])


def parse_args():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--docker", default="docker")
    parser.add_argument("--image", required=True, help="local image reference")
    parser.add_argument("--arch", required=True, choices=sorted(ELF_MACHINE))
    parser.add_argument("--max-image-bytes", type=int, required=True)
    parser.add_argument("--max-binary-bytes", type=int, required=True)
    return parser.parse_args()


def main():
    args = parse_args()
    try:
        total, app_size, head = measure(args.docker, args.image)
        self_test(args.docker, args.image, args.arch, total, app_size, head)
        failures = (check_run(args.docker, args.image, args.arch) + check_user(args.docker, args.image)
                    + check_arch(head, args.arch)
                    + check_size(total, app_size, args.max_image_bytes, args.max_binary_bytes))
        if failures:
            raise SmokeError("; ".join(failures))
    except SmokeError as err:
        print(f"Image smoke test failed: {err}", file=sys.stderr)
        return 1
    print(f"Image smoke test: {args.image} ({args.arch}) runs, user {USER}, "
          f"image files {total:,} bytes (limit {args.max_image_bytes:,}), "
          f"/app {app_size:,} bytes (limit {args.max_binary_bytes:,})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
