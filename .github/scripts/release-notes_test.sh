#!/bin/sh
# Self-test of release-notes.sh: it must print a version's section and must
# fail for everything else. Run from the repository root.
set -eu

script=.github/scripts/release-notes.sh
dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT
log="$dir/CHANGELOG.md"
printf '%s\n' '# Changelog' '' '## [Unreleased]' '' \
  '## [0.2.0] - 2026-11-01' '' '### Fixed' '' '- Fix a thing' '' \
  '## [0.1.0] - 2026-10-10' '' '### Added' '' '- Add one' '- Add two' '' \
  '## [0.0.9] - 2026-01-01' > "$log"

failures=0
fail() {
  echo "FAIL: $1" >&2
  failures=$((failures + 1))
}

want=$(printf '%s\n' '### Added' '' '- Add one' '- Add two')
got=$(sh "$script" 0.1.0 "$log") || fail "0.1.0 exits non-zero"
[ "$got" = "$want" ] || fail "0.1.0 prints the wrong section"

got=$(sh "$script" 0.2.0 "$log") || fail "0.2.0 exits non-zero"
case "$got" in
  *"Add one"*) fail "0.2.0 runs into the next section" ;;
esac

# Each of these must be refused: no section, an empty section, a version with
# a prefix, a shorter version that is a prefix of a real one, text after the
# version, a missing file, and a missing argument.
for version in 0.3.0 0.0.9 v0.1.0 0.1 '0.1.0]x' ''; do
  if sh "$script" "$version" "$log" > /dev/null 2>&1; then
    fail "version '$version' is accepted"
  fi
done
if sh "$script" 0.1.0 "$dir/none.md" > /dev/null 2>&1; then
  fail "a missing file is accepted"
fi
if sh "$script" 0.1.0 > /dev/null 2>&1; then
  fail "a missing argument is accepted"
fi

# release-image-note.sh prints the block that names the image of a release by
# tag and by digest. It must refuse everything that is not an image name, a
# version X.Y.Z and a sha256 digest.
note=.github/scripts/release-image-note.sh
digest=sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
# The backticks are Markdown, not a command.
# shellcheck disable=SC2016
want=$(printf '%s\n' '' '## Image' '' \
  '- `ghcr.io/example/app:0.1.0`' \
  "- \`ghcr.io/example/app@$digest\`" '' \
  'Verify the image before you use it: README, "Verifying a release".')
got=$(sh "$note" ghcr.io/example/app 0.1.0 "$digest") ||
  fail "image note exits non-zero"
[ "$got" = "$want" ] || fail "image note prints the wrong block"

short=${digest%?}
for bad in "$short" "${digest}0" "SHA256:${digest#sha256:}" \
  "${digest#sha256:}" "sha256:$(printf '%064d' 0 | tr 0 G)" ''; do
  if sh "$note" ghcr.io/example/app 0.1.0 "$bad" > /dev/null 2>&1; then
    fail "digest '$bad' is accepted"
  fi
done
for version in v0.1.0 0.1 '' '0.1.0 x'; do
  if sh "$note" ghcr.io/example/app "$version" "$digest" > /dev/null 2>&1; then
    fail "image note accepts version '$version'"
  fi
done
for image in '' 'ghcr.io/Example/app' 'ghcr.io/example/app:1' 'a b'; do
  if sh "$note" "$image" 0.1.0 "$digest" > /dev/null 2>&1; then
    fail "image note accepts image '$image'"
  fi
done
if sh "$note" ghcr.io/example/app 0.1.0 > /dev/null 2>&1; then
  fail "image note accepts a missing argument"
fi

if [ "$failures" -ne 0 ]; then
  echo "release-notes self-test: $failures failure(s)" >&2
  exit 1
fi
echo "release-notes self-test: ok"
