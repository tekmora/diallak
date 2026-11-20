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
COMMENTS = re.compile(r'<!--.*?-->', re.DOTALL)
TASK_LINE = re.compile(r'^ *[-*+] \[[ xX]\][ \t]+\S', re.MULTILINE)
FENCE = re.compile(r'^ {0,3}(`{3,}|~{3,})')


def read_input(path):
    with path.open('rb') as source:
        data = source.read(MAX_INPUT_BYTES + 1)
    if len(data) > MAX_INPUT_BYTES:
        raise ValueError(f'{path}: input exceeds {MAX_INPUT_BYTES} bytes')
    return data.decode('utf-8')


def sections(text):
    result = []
    fence = ''
    for line in text.splitlines():
        marker = FENCE.match(line)
        if fence:
            if (marker and marker.group(1)[0] == fence[0]
                    and len(marker.group(1)) >= len(fence)
                    and not line[marker.end():].strip(' ')):
                fence = ''
            continue
        if marker:
            fence = marker.group(1)
            continue
        if line.startswith('## '):
            result.append((line[3:].strip(), []))
        elif result:
            result[-1][1].append(line)
    return result


def findings(body, template):
    body = COMMENTS.sub(' ', body)
    template = COMMENTS.sub(' ', template)
    template_sections = sections(template)
    headings = [name for name, _ in template_sections]
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
        description = content[headings[0]]
        if len(description.split()) < MIN_DESCRIPTION_WORDS:
            found.append(('description-empty', f'description needs at least {MIN_DESCRIPTION_WORDS} words'))
    for name, lines in template_sections:
        template_content = '\n'.join(lines)
        body_content = content.get(name, '')
        if TASK_LINE.search(template_content) and name in content and not TASK_LINE.search(body_content):
            found.append(('checklist-empty', f'checklist needs a task-list item: {name}'))
    return found


def cases(template):
    template_sections = sections(COMMENTS.sub(' ', template))
    description = f'## {template_sections[0][0]}\n'
    checklist_names = [name for name, lines in template_sections
                       if TASK_LINE.search('\n'.join(lines))]
    checklist = ''.join(f'## {name}\n' + ('- [ ] Check\n' if name in checklist_names else '')
                        for name, _ in template_sections[1:])
    empty_checklist = checklist.replace('- [ ] Check\n', '', 1)
    table = '| Package | From | To |\n| --- | --- | --- |\n| example | 1 | 2 |\n'
    renovate = ('This PR contains the following updates:\n' + table +
                '### Release Notes\n<details>Notes</details>\n')
    template_lines = template.splitlines(keepends=True)
    first_heading = next(i for i, line in enumerate(template_lines)
                         if line.rstrip('\r\n') == description.rstrip('\n'))
    template_lines.insert(first_heading + 1, 'This explains the change.\n')
    filled = ''.join(template_lines)
    missing = ['section-missing'] * len(template_sections)
    return [
        ('renovate-style', renovate, missing),
        ('committed template', template, ['description-empty']),
        ('filled template', filled, []),
        ('star task', description + 'Three useful words\n' + checklist.replace('- [ ] Check', '* [ ] Check'), []),
        ('plus task', description + 'Three useful words\n' + checklist.replace('- [ ] Check', '+ [ ] Check'), []),
        ('no space after box', description + 'Three useful words\n' + checklist.replace('- [ ] Check', '- [ ]not a task'), ['checklist-empty']),
        ('bare box', description + 'Three useful words\n' + checklist.replace('- [ ] Check', '- [ ]'), ['checklist-empty']),
        ('commented heading', description + '<!--\n## Internal notes\n-->\nThree useful words\n' + checklist, []),
        ('update table', description + table + checklist, []),
        ('empty checklist', description + 'Three useful words\n' + empty_checklist, ['checklist-empty']),
        ('reverse order', checklist + description + 'Three useful words\n', ['section-order']),
        ('two words', description + 'Two words\n' + checklist, ['description-empty']),
        ('three words', description + 'Exactly three words\n' + checklist, []),
        ('empty body', '', missing),
        ('filled template CRLF', filled.replace('\n', '\r\n'), []),
        ('renovate-style CRLF', renovate.replace('\n', '\r\n'), missing),
    ]


def self_test(template):
    planted = cases(template)
    failures = 0
    for name, body, expected in planted:
        actual = [rule for rule, _ in findings(body, template)]
        if sorted(actual) != sorted(expected):
            print(f'PR body self-test: {name} failed: expected {expected}, got {actual}')
            failures += 1
    two_checklists = ('## Description\nTemplate description\n'
                      '## First checks\n- [ ] First\n'
                      '## Second checks\n- [ ] Second\n')
    body = ('## Description\nThree useful words\n'
            '## First checks\n- [ ] Done\n## Second checks\n')
    expected = [('checklist-empty', 'checklist needs a task-list item: Second checks')]
    actual = findings(body, two_checklists)
    if actual != expected:
        print(f'PR body self-test: two checklists failed: expected {expected}, got {actual}')
        failures += 1
    fence_template = '## Description\n## Checks\n- [ ] Verify\n'
    valid_body = '## Description\nThree useful words\n## Checks\n- [ ] Done\n'
    fence_cases = [
        ('fenced headings', '```markdown\n' + valid_body + '```\n', fence_template,
         ['section-missing', 'section-missing']),
        ('fenced task', '## Description\nThree useful words\n## Checks\n```\n- [ ] Done\n```\n',
         fence_template, ['checklist-empty']),
        ('tilde fence', '~~~\n' + valid_body + '~~~\n', fence_template,
         ['section-missing', 'section-missing']),
        ('unclosed fence', '```\n' + valid_body, fence_template,
         ['section-missing', 'section-missing']),
        ('short close', '````\n```\n' + valid_body, fence_template,
         ['section-missing', 'section-missing']),
        ('other close', '```\n~~~\n' + valid_body, fence_template,
         ['section-missing', 'section-missing']),
        ('after close', '   ```\nignored\n   ````  \n' + valid_body, fence_template, []),
        ('close with text', '```\n``` text\n' + valid_body, fence_template,
         ['section-missing', 'section-missing']),
        ('fenced description', '## Description\n```\nThree useful words\n```\n'
         '## Checks\n- [ ] Done\n', fence_template, ['description-empty']),
        ('template fenced task', '## Description\nThree useful words\n## Checks\n',
         '## Description\n## Checks\n```\n- [ ] Example\n```\n', []),
        ('template fenced heading', '## Description\nThree useful words\n',
         '## Description\n```\n## Example\n```\n', []),
    ]
    for name, fence_body, fence_source, expected_rules in fence_cases:
        actual_rules = [rule for rule, _ in findings(fence_body, fence_source)]
        if sorted(actual_rules) != sorted(expected_rules):
            print(f'PR body self-test: {name} failed: expected {expected_rules}, got {actual_rules}')
            failures += 1
    if failures:
        return 1
    print(f'PR body self-test: {len(planted) + len(fence_cases) + 1} cases behaved as required')
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
