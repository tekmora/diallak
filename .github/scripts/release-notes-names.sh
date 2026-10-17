#!/bin/sh
# Verifies that no supporter name from a names file appears in a notes file.
# usage: release-notes-names.sh <notes-file> <names-file>
# Exits 1 when any name appears in the notes file, or when arguments or files
# are missing. Exits 0 otherwise.
set -eu

if [ "$#" -ne 2 ]; then
  echo "usage: release-notes-names.sh <notes-file> <names-file>" >&2
  exit 1
fi
notes_file=$1
names_file=$2

if [ ! -f "$notes_file" ]; then
  echo "release-notes-names: no such file: $notes_file" >&2
  exit 1
fi
if [ ! -f "$names_file" ]; then
  echo "release-notes-names: no such file: $names_file" >&2
  exit 1
fi

tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT

tr -d '\r' < "$names_file" | awk '
  {
    sub(/^[[:space:]]+/, "")
    sub(/[[:space:]]+$/, "")
    if ($0 == "" || substr($0, 1, 1) == "#") next
    gsub(/[[:space:]]+/, " ")
    print
  }
' > "$tmp"

if [ ! -s "$tmp" ]; then
  exit 0
fi

if ! grep -F -q -w -i -f "$tmp" "$notes_file"; then
  exit 0
fi

matched_lines=$(grep -F -w -i -n -f "$tmp" "$notes_file" | cut -d: -f1)

count=0
for num in $matched_lines; do
  echo "release-notes-names: hit on line $num" >&2
  count=$((count + 1))
done
echo "release-notes-names: $count hit(s)" >&2
exit 1
