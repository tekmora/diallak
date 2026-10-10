#!/bin/sh
# Prints the section of a Keep a Changelog file for one version.
# usage: release-notes.sh <X.Y.Z> <CHANGELOG.md>
# Exits 1 when the arguments are wrong, the file is missing, or the file has
# no section "## [X.Y.Z]" with at least one line of content.
set -eu

if [ "$#" -ne 2 ]; then
  echo "usage: release-notes.sh <X.Y.Z> <changelog file>" >&2
  exit 1
fi
version=$1
file=$2

case "$version" in
  *[!0-9.]* | "" | .* | *. | *..*)
    echo "release-notes: version must look like X.Y.Z, got '$version'" >&2
    exit 1
    ;;
esac
if [ ! -f "$file" ]; then
  echo "release-notes: no such file: $file" >&2
  exit 1
fi

notes=$(awk -v want="## [$version]" '
  index($0, want) == 1 { found = 1; next }
  found && index($0, "## [") == 1 { exit }
  found { print }
' "$file")

# Leading and trailing blank lines carry nothing.
notes=$(printf '%s\n' "$notes" | sed -e '/./,$!d' | sed -e :a -e '/^\n*$/{$d;N;ba' -e '}')
if [ -z "$notes" ]; then
  echo "release-notes: $file has no section for version $version" >&2
  exit 1
fi
printf '%s\n' "$notes"
