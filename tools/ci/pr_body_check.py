"""Check that a pull request description fills the template's sections.

The self-test replays missing, empty, ordered, and filled sections before CI trusts
this check on a pull request body.
"""

import argparse
import os
import re
import sys
from pathlib import Path

MAX_INPUT_BYTES = 262144
MIN_DESCRIPTION_WORDS = 3
DEFAULT_TEMPLATE = Path('.github/pull_request_template.md')
CHECKLIST_HEADING = 'Pre-Merge Verification Checklist'
COMMENTS = re.compile(r'<!--.*?-->', re.DOTALL)
TASK_LINE = re.compile(r'^ *- \[[ xX]\]', re.MULTILINE)


def read_input(path):
    with path.open('rb') as source:
        data = source.read(MAX_INPUT_BYTES + 1)
    if len(data) > MAX_INPUT_BYTES:
        raise ValueError(f'{path}: input exceeds {MAX_INPUT_BYTES} bytes')
    return data.decode('utf-8')


def sections(text):
    result = []
    for line in text.splitlines():
        if line.startswith('## '):
            result.append((line[3:].strip(), []))
        elif result:
            result[-1][1].append(line)
    return result


def findings(body, template):
    headings = [name for name, _ in sections(template)]
    if not headings:
        raise ValueError('template has no level-two headings')
    body_sections = sections(body)
    present = [name for name, _ in body_sections]
    found = [('section-missing', f'missing heading: {name}')
             for name in headings if name not in present]
    indices = [headings.index(name) for name in present if name in headings]
    if indices != sorted(indices):
        found.append(('section-order', 'template headings are out of order'))
    content = {name: '\n'.join(lines) for name, lines in reversed(body_sections)}
    if headings[0] in content:
        description = COMMENTS.sub(' ', content[headings[0]])
        if len(description.split()) < MIN_DESCRIPTION_WORDS:
            found.append(('description-empty', 'description needs at least 3 words'))
    if CHECKLIST_HEADING in content and not TASK_LINE.search(content[CHECKLIST_HEADING]):
        found.append(('checklist-empty', 'verification checklist needs a task-list item'))
    return found


def cases(template):
    description = '## Description\n'
    checklist = f'## {CHECKLIST_HEADING}\n- [ ] Check\n'
    table = '| Package | From | To |\n| --- | --- | --- |\n| example | 1 | 2 |\n'
    return [
        ('renovate-style', 'This PR contains the following updates:\n' + table +
         '### Release Notes\n<details>Notes</details>\n', ['section-missing', 'section-missing']),
        ('committed template', template, ['description-empty']),
        ('filled template', description + 'This explains the change. It provides context.\n' + checklist, []),
        ('update table', description + table + checklist, []),
        ('empty checklist', description + 'Three useful words\n' + f'## {CHECKLIST_HEADING}\n', ['checklist-empty']),
        ('reverse order', checklist + description + 'Three useful words\n', ['section-order']),
        ('two words', description + 'Two words\n' + checklist, ['description-empty']),
        ('three words', description + 'Exactly three words\n' + checklist, []),
        ('empty body', '', ['section-missing', 'section-missing']),
    ]


def self_test(template):
    planted = cases(template)
    failures = 0
    for name, body, expected in planted:
        actual = [rule for rule, _ in findings(body, template)]
        if sorted(actual) != sorted(expected):
            print(f'PR body self-test: {name} failed: expected {expected}, got {actual}')
            failures += 1
    if failures:
        return 1
    print(f'PR body self-test: {len(planted)} cases behaved as required')
    return 0


def report(found):
    if not found:
        print('pr-body: description sections present')
        return 0
    actions = os.environ.get('GITHUB_ACTIONS') == 'true'
    for rule, detail in found:
        print(f'pr-body: {rule}: {detail}')
        if actions:
            escaped = detail.replace('%', '%25').replace('\r', '%0D').replace('\n', '%0A')
            print(f'::error title=PR description::{rule}: {escaped}')
    summary = os.environ.get('GITHUB_STEP_SUMMARY') if actions else None
    if summary:
        with open(summary, 'a', encoding='utf-8') as output:
            output.write('### PR description findings\n\n')
            for rule, detail in found:
                output.write(f'- `{rule}`: {detail}\n')
    return 1


def main(arguments=None):
    parser = argparse.ArgumentParser(description=__doc__)
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument('--body-file', type=Path)
    source.add_argument('--body-env')
    source.add_argument('--self-test', action='store_true')
    parser.add_argument('--template', type=Path, default=DEFAULT_TEMPLATE)
    options = parser.parse_args(arguments)
    try:
        template = read_input(options.template)
        if options.self_test:
            return self_test(template)
        body = read_input(options.body_file) if options.body_file else os.environ.get(options.body_env, '')
        if len(body.encode('utf-8')) > MAX_INPUT_BYTES:
            raise ValueError(f'body exceeds {MAX_INPUT_BYTES} bytes')
        return report(findings(body, template))
    except (OSError, UnicodeError, ValueError) as error:
        print(f'pr-body: {error}', file=sys.stderr)
        return 2


if __name__ == '__main__':
    sys.exit(main())
