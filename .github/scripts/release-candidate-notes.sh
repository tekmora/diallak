#!/bin/sh
# Prints the release notes of a release candidate from the changelog
# fragments that are not yet rendered into CHANGELOG.md, grouped by type in
# the order of Keep a Changelog.
# usage: release-candidate-notes.sh <X.Y.Z-rc.N> <fragment folder>
# Exits 1 when the arguments are wrong, the version is not X.Y.Z-rc.N, the
# folder holds no fragment, or a fragment has an unknown type or no title.
set -eu

if [ "$#" -ne 2 ]; then
  echo "usage: release-candidate-notes.sh <X.Y.Z-rc.N> <fragment folder>" >&2
  exit 1
fi
version=$1
dir=$2

# grep reads lines, so a version with a line break is refused first.
case "$version" in
  *[!0-9a-z.-]*) version=bad ;;
esac
if ! printf '%s\n' "$version" |
  grep -Eqx '[0-9]+\.[0-9]+\.[0-9]+-rc\.[0-9]+'; then
  echo "release-candidate-notes: version must look like X.Y.Z-rc.N," \
    "got '$version'" >&2
  exit 1
fi
if [ ! -d "$dir" ]; then
  echo "release-candidate-notes: no such folder: $dir" >&2
  exit 1
fi
set -- "$dir"/*.yaml
if [ ! -f "$1" ]; then
  echo "release-candidate-notes: $dir holds no fragment" >&2
  exit 1
fi

# One line per fragment: its type, a tab, its title. A single-quoted title
# has its doubled quotes undone.
entries=$(awk '
  FNR == 1 && NR > 1 { emit() }
  /^type: / { type = substr($0, 7) }
  /^title: / {
    title = substr($0, 8)
    if (title ~ /^\047.*\047$/) {
      title = substr(title, 2, length(title) - 2)
      gsub(/\047\047/, "\047", title)
    }
  }
  END { if (!failed) emit() }
  function emit() {
    if (type !~ /^(added|changed|deprecated|removed|fixed|security)$/) {
      printf "release-candidate-notes: %s: unknown type \047%s\047\n", \
        prev, type > "/dev/stderr"
      failed = 1
      exit 1
    }
    if (title == "") {
      printf "release-candidate-notes: %s: no title\n", prev > "/dev/stderr"
      failed = 1
      exit 1
    }
    printf "%s\t%s\n", type, title
    type = ""; title = ""
  }
  { prev = FILENAME }
' "$@")

printf 'Release candidate %s, for testing only. It is not a release: the\n' \
  "$version"
printf 'changes below are not yet in CHANGELOG.md.\n'
for type in added changed deprecated removed fixed security; do
  lines=$(printf '%s\n' "$entries" | awk -F '\t' -v t="$type" \
    '$1 == t { print "- " $2 }')
  if [ -n "$lines" ]; then
    heading=$(printf '%s' "$type" | awk '{ print toupper(substr($0, 1, 1)) substr($0, 2) }')
    printf '\n### %s\n\n%s\n' "$heading" "$lines"
  fi
done
