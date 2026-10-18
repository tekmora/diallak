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

# A release candidate X.Y.Z-rc.N carries the image tag of its full version.
got=$(sh "$note" ghcr.io/example/app 0.5.0-rc.1 "$digest") ||
  fail "image note refuses a release candidate"
# shellcheck disable=SC2016
case "$got" in
  *'`ghcr.io/example/app:0.5.0-rc.1`'*) ;;
  *) fail "image note does not name the release candidate tag" ;;
esac
nl='
'
for version in 0.5.0-rc 0.5.0-rc. 0.5.0-rc.x 0.5.0-beta.1 0.5.0-rc.1.2 \
  0.5-rc.1 0.5.0rc.1 "0.5.0-rc.1${nl}x"; do
  if sh "$note" ghcr.io/example/app "$version" "$digest" > /dev/null 2>&1; then
    fail "image note accepts version '$version'"
  fi
done

# release-candidate-notes.sh prints the notes of a release candidate from the
# fragments not yet rendered into the changelog, grouped by type in the order
# of Keep a Changelog. It must refuse every version that is not X.Y.Z-rc.N, a
# missing or empty fragment folder, an unknown type and a fragment without a
# title.
rc=.github/scripts/release-candidate-notes.sh
frag="$dir/changelog.d"
mkdir "$frag"
printf '%s\n' 'type: fixed' "title: 'Fix the bot''s thing'" > "$frag/A-fix.yaml"
printf '%s\n' 'type: added' "title: 'Add one'" > "$frag/B-add.yaml"
printf '%s\n' 'type: added' 'title: Add two' > "$frag/C-add.yaml"
want=$(printf '%s\n' \
  'Release candidate 0.5.0-rc.1, for testing only. It is not a release: the' \
  'changes below are not yet in CHANGELOG.md.' '' \
  '### Added' '' '- Add one' '- Add two' '' \
  '### Fixed' '' "- Fix the bot's thing")
got=$(sh "$rc" 0.5.0-rc.1 "$frag") || fail "candidate notes exit non-zero"
[ "$got" = "$want" ] || fail "candidate notes print the wrong text"
for version in 0.5.0 v0.5.0-rc.1 0.5.0-rc 0.5.0-rc.x 0.5.0-beta.1 \
  0.5-rc.1 '0.5.0-rc.1 x' "0.5.0-rc.1${nl}x" ''; do
  if sh "$rc" "$version" "$frag" > /dev/null 2>&1; then
    fail "candidate notes accept version '$version'"
  fi
done
# The two refusals below name their reason, so that the release run says why
# it stopped.
mkdir "$dir/empty"
if err=$(sh "$rc" 0.5.0-rc.1 "$dir/empty" 2>&1 > /dev/null); then
  fail "candidate notes accept an empty fragment folder"
fi
case "$err" in
  *"holds no fragment"*) ;;
  *) fail "candidate notes do not say the folder holds no fragment" ;;
esac
if err=$(sh "$rc" 0.5.0-rc.1 "$dir/none" 2>&1 > /dev/null); then
  fail "candidate notes accept a missing fragment folder"
fi
case "$err" in
  *"no such folder"*) ;;
  *) fail "candidate notes do not say the folder is missing" ;;
esac
if sh "$rc" 0.5.0-rc.1 > /dev/null 2>&1; then
  fail "candidate notes accept a missing argument"
fi
printf '%s\n' 'type: improved' "title: 'x'" > "$frag/D-bad.yaml"
if sh "$rc" 0.5.0-rc.1 "$frag" > /dev/null 2>&1; then
  fail "candidate notes accept an unknown type"
fi
printf '%s\n' 'type: added' > "$frag/D-bad.yaml"
if sh "$rc" 0.5.0-rc.1 "$frag" > /dev/null 2>&1; then
  fail "candidate notes accept a fragment without a title"
fi

# release-credits-note.sh prints the release note block linking the credits page.
# It must refuse missing/empty arguments, anything not starting with https://,
# and URLs with whitespace, backticks, <, >, (, or ).
credits_note=.github/scripts/release-credits-note.sh
url="https://example.com/credits"
want=$(printf '%s\n' '' '## Supporters' '' \
  'Thanks to the people who back this project. See the [credits page](https://example.com/credits).')
got=$(sh "$credits_note" "$url") || fail "credits note exits non-zero"
[ "$got" = "$want" ] || fail "credits note prints the wrong block"

for bad in 'http://example.com/credits' '' 'https://a b' 'https://x)' 'https://x(' \
  'https://x`y' 'https://x<y' 'https://x>y'; do
  if sh "$credits_note" "$bad" > /dev/null 2>&1; then
    fail "credits note accepts invalid url '$bad'"
  fi
done
if sh "$credits_note" > /dev/null 2>&1; then
  fail "credits note accepts missing argument"
fi

# release-notes-names.sh exits 1 when any supporter name from a names file
# appears in a notes file, and 0 otherwise.
names_script=.github/scripts/release-notes-names.sh
names_file="$dir/supporters.txt"
notes_file="$dir/notes.md"

printf '%s\n' 'Planted Supporter' > "$names_file"
printf '%s\n' '### Added' '' '- Refuse a long question for Planted Supporter' > "$notes_file"
if sh "$names_script" "$notes_file" "$names_file" > /dev/null 2>&1; then
  fail "planted name in CHANGELOG-style section is accepted"
fi

printf '%s\n' '### Added' '' '- Refuse a long question for planted supporter' > "$notes_file"
if sh "$names_script" "$notes_file" "$names_file" > /dev/null 2>&1; then
  fail "differently cased name is accepted"
fi

printf '%s\n' 'Ann' > "$names_file"
printf '%s\n' '### Added' '' '- Annual report of changes' > "$notes_file"
sh "$names_script" "$notes_file" "$names_file" > /dev/null 2>&1 ||
  fail "name as part of longer word is refused"

printf '%s\n' 'Planted Supporter' > "$names_file"
printf '%s\n' '### Added' '' '- Thanks to Planted Supporters today' > "$notes_file"
sh "$names_script" "$notes_file" "$names_file" > /dev/null 2>&1 ||
  fail "name as prefix of longer word is refused"

printf '%s\n' 'Planted Supporter' > "$names_file"
printf '%s\n' '### Added' '' '- Thanks to Planted Supporter today' > "$notes_file"
if sh "$names_script" "$notes_file" "$names_file" > /dev/null 2>&1; then
  fail "two-word name is accepted"
fi

printf '%s\n' '' '   # Comment line' '' '  # Another comment' '  ' > "$names_file"
printf '%s\n' '### Added' '' '- Clean note without any names' > "$notes_file"
sh "$names_script" "$notes_file" "$names_file" > /dev/null 2>&1 ||
  fail "blank line and comment in names file with clean notes is refused"

: > "$names_file"
sh "$names_script" "$notes_file" "$names_file" > /dev/null 2>&1 ||
  fail "empty names file with clean notes is refused"

if sh "$names_script" "$notes_file" "$dir/nonexistent.txt" > /dev/null 2>&1; then
  fail "missing names file is accepted"
fi
if sh "$names_script" "$dir/nonexistent.md" "$names_file" > /dev/null 2>&1; then
  fail "missing notes file is accepted"
fi
if sh "$names_script" "$notes_file" > /dev/null 2>&1; then
  fail "missing argument is accepted"
fi
if sh "$names_script" > /dev/null 2>&1; then
  fail "missing arguments are accepted"
fi

printf '%s\n' 'Planted Supporter' > "$names_file"
printf '%s\n' 'Line 1: clean' 'Line 2: SensitiveText Planted Supporter here' 'Line 3: clean' > "$notes_file"
hit_err=
if hit_err=$(sh "$names_script" "$notes_file" "$names_file" 2>&1 > /dev/null); then
  fail "hit exits zero"
fi
case "$hit_err" in
  *"Planted"*|*"Supporter"*) fail "stderr contains the supporter name" ;;
  *"SensitiveText"*|*"Line 2:"*) fail "stderr contains line text" ;;
  *"2"*) ;;
  *) fail "stderr does not contain line number 2" ;;
esac
case "$hit_err" in
  *"1 hit"*) ;;
  *) fail "stderr does not report 1 hit" ;;
esac

if [ "$failures" -ne 0 ]; then
  echo "release-notes self-test: $failures failure(s)" >&2
  exit 1
fi
echo "release-notes self-test: ok"
