#!/bin/sh
# Prints the release note block that thanks supporters and links the credits
# page.
# usage: release-credits-note.sh <url>
# An empty argument prints nothing and exits 0. Exits 1 when the argument is
# missing, does not start with https://, or contains whitespace, backticks, <,
# >, (, or ).
set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: release-credits-note.sh <url>" >&2
  exit 1
fi
url=$1

if [ -z "$url" ]; then
  exit 0
fi

case "$url" in
  https://*) ;;
  *)
    echo "release-credits-note: url must start with https://, got '$url'" >&2
    exit 1
    ;;
esac

case "$url" in
  *[[:space:]\`\<\>\(\)]*)
    echo "release-credits-note: url contains invalid characters: '$url'" >&2
    exit 1
    ;;
esac

printf '\n## Supporters\n\n'
printf 'Thanks to the people who back this project. See the [credits page](%s).\n' "$url"
