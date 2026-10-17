#!/bin/sh
# Prints the release note block that names the image of a release by tag and
# by digest.
# usage: release-image-note.sh <image> <X.Y.Z[-rc.N]> <sha256:digest>
# Exits 1 when an argument is missing or is not an image name without a tag,
# a version X.Y.Z or X.Y.Z-rc.N, or a sha256 digest of 64 lowercase
# hexadecimal digits.
set -eu

if [ "$#" -ne 3 ]; then
  echo "usage: release-image-note.sh <image> <X.Y.Z[-rc.N]> <sha256:digest>" >&2
  exit 1
fi
image=$1
version=$2
digest=$3

case "$image" in
  "" | *[!a-z0-9./-]*)
    echo "release-image-note: not an image name without a tag: '$image'" >&2
    exit 1
    ;;
esac
# grep reads lines, so a version with a line break is refused first.
case "$version" in
  *[!0-9a-z.-]*) version=bad ;;
esac
if ! printf '%s\n' "$version" |
  grep -Eqx '[0-9]+\.[0-9]+\.[0-9]+(-rc\.[0-9]+)?'; then
  echo "release-image-note: version must look like X.Y.Z or X.Y.Z-rc.N," \
    "got '$version'" >&2
  exit 1
fi
hex=${digest#sha256:}
case "$hex" in
  *[!0-9a-f]*) hex= ;;
esac
if [ "$hex" = "$digest" ] || [ "${#hex}" -ne 64 ]; then
  echo "release-image-note: not a sha256 digest: '$digest'" >&2
  exit 1
fi

printf '\n## Image\n\n'
# The backticks are Markdown, not a command: the format stays in single quotes.
# shellcheck disable=SC2016
printf -- '- `%s:%s`\n' "$image" "$version"
# shellcheck disable=SC2016
printf -- '- `%s@%s`\n' "$image" "$digest"
printf '\nVerify the image before you use it: README, "Verifying a release".\n'
