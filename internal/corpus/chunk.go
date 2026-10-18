package corpus

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

type rawChunk struct {
	heading string
	lines   []string
}

type lineKind int

const (
	kindNormal lineKind = iota
	kindHeading
	kindSetextUnderline
	kindVerbatim
)

type headingInfo struct {
	level int
	text  string
}

func stripHTMLComments(line string, inComment bool) (string, bool) {
	var sb strings.Builder
	pos := 0
	n := len(line)
	for pos < n {
		if inComment {
			idx := strings.Index(line[pos:], "-->")
			if idx == -1 {
				return sb.String(), true
			}
			pos += idx + 3
			inComment = false
		} else {
			idx := strings.Index(line[pos:], "<!--")
			if idx == -1 {
				sb.WriteString(line[pos:])
				break
			}
			sb.WriteString(line[pos : pos+idx])
			pos += idx + 4
			inComment = true
		}
	}
	return sb.String(), inComment
}

func splitBody(body string, maxBytes int) []string {
	lines := strings.Split(body, "\n")
	var paras []string
	var curPara []string
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			if len(curPara) > 0 {
				paras = append(paras, strings.Join(curPara, "\n"))
				curPara = nil
			}
		} else {
			curPara = append(curPara, l)
		}
	}
	if len(curPara) > 0 {
		paras = append(paras, strings.Join(curPara, "\n"))
	}
	bounded := cutOversized(paras, maxBytes)
	return groupParagraphs(bounded, maxBytes)
}

func cutOversized(paras []string, maxBytes int) []string {
	var bounded []string
	for _, p := range paras {
		limit := len(p)/maxBytes + 2
		for i := 0; i < limit && len(p) > maxBytes; i++ {
			cut := maxBytes
			for cut > 0 && !utf8.RuneStart(p[cut]) {
				cut--
			}
			bounded = append(bounded, strings.TrimSpace(p[:cut]))
			p = strings.TrimSpace(p[cut:])
		}
		if len(p) > 0 {
			bounded = append(bounded, p)
		}
	}
	return bounded
}

func groupParagraphs(paras []string, maxBytes int) []string {
	var parts []string
	var cur string
	for _, p := range paras {
		if cur == "" {
			cur = p
			continue
		}
		cand := cur + "\n\n" + p
		if len(cand) <= maxBytes {
			cur = cand
		} else {
			parts = append(parts, cur)
			cur = p
		}
	}
	if cur != "" {
		parts = append(parts, cur)
	}
	return parts
}

// fenceRun returns the run of fence characters a trimmed line starts with: three or more
// backticks or tildes, or "" when the line starts no fence.
func fenceRun(trimmed string) string {
	if len(trimmed) < 3 || (trimmed[0] != '`' && trimmed[0] != '~') {
		return ""
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == trimmed[0] {
		n++
	}
	if n < 3 {
		return ""
	}
	return trimmed[:n]
}

// closesFence reports whether a trimmed line closes the fence opened by open: CommonMark
// wants the same character, a run at least as long, and nothing after it. A shorter run or
// the other character is code inside the block.
func closesFence(trimmed, open string) bool {
	run := fenceRun(trimmed)
	return run != "" && run[0] == open[0] && len(run) >= len(open) && len(run) == len(trimmed)
}

// lineOffsets builds the starting byte offset for every line in lines.
func lineOffsets(lines []string) []int {
	starts := make([]int, len(lines))
	offset := 0
	for i := 0; i < len(lines); i++ {
		starts[i] = offset
		offset += len(lines[i]) + 1
	}
	return starts
}

// offsetToLine finds the line index containing byte offset pos.
func offsetToLine(starts []int, pos int) int {
	if pos <= 0 || len(starts) == 0 {
		return 0
	}
	idx := sort.Search(len(starts), func(i int) bool {
		return starts[i] > pos
	}) - 1
	if idx < 0 {
		return 0
	}
	if idx >= len(starts) {
		return len(starts) - 1
	}
	return idx
}

// headingText extracts the text of an AST heading, HTML comments removed and trimmed.
func headingText(h *ast.Heading, src []byte) string {
	var parts []string
	lines := h.Lines()
	limit := lines.Len()
	inComment := false
	for i := 0; i < limit; i++ {
		seg := lines.At(i)
		var stripped string
		stripped, inComment = stripHTMLComments(string(seg.Value(src)), inComment)
		parts = append(parts, strings.TrimSpace(stripped))
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

// markHeading records line actions and heading info for a heading node.
func markHeading(h *ast.Heading, src []byte, starts []int, lines []string, actions []lineKind, headings map[int]headingInfo) {
	startLine := offsetToLine(starts, h.Pos())
	text := headingText(h, src)
	actions[startLine] = kindHeading
	headings[startLine] = headingInfo{level: h.Level, text: text}

	limit := h.Lines().Len()
	for i := 1; i < limit && startLine+i < len(actions); i++ {
		actions[startLine+i] = kindHeading
	}
	if limit > 0 {
		underlineLine := startLine + limit
		if underlineLine < len(lines) && actions[underlineLine] == kindNormal {
			trimmed := strings.TrimSpace(lines[underlineLine])
			if len(trimmed) > 0 && (trimmed[0] == '=' || trimmed[0] == '-') {
				actions[underlineLine] = kindSetextUnderline
			}
		}
	}
}

// markFencedCodeBlock records verbatim line actions for a fenced code block node.
func markFencedCodeBlock(fcb *ast.FencedCodeBlock, starts []int, lines []string, actions []lineKind) {
	startLine := offsetToLine(starts, fcb.Pos())
	actions[startLine] = kindVerbatim
	limit := fcb.Lines().Len()
	for i := 0; i < limit; i++ {
		seg := fcb.Lines().At(i)
		actions[offsetToLine(starts, seg.Start)] = kindVerbatim
	}
	closeLine := startLine + 1 + limit
	if closeLine < len(lines) {
		openTrimmed := strings.TrimSpace(lines[startLine])
		trimmed := strings.TrimSpace(lines[closeLine])
		if closesFence(trimmed, fenceRun(openTrimmed)) {
			actions[closeLine] = kindVerbatim
		}
	}
}

// classifyLines maps AST block nodes to per-line handling actions.
func classifyLines(doc ast.Node, src []byte, starts []int, lines []string) ([]lineKind, map[int]headingInfo) {
	actions := make([]lineKind, len(lines))
	headings := make(map[int]headingInfo)
	maxChildren := doc.ChildCount() + 1
	for child, i := doc.FirstChild(), 0; child != nil && i < maxChildren; child, i = child.NextSibling(), i+1 {
		switch n := child.(type) {
		case *ast.Heading:
			markHeading(n, src, starts, lines, actions, headings)
		case *ast.FencedCodeBlock:
			markFencedCodeBlock(n, starts, lines, actions)
		}
	}
	return actions, headings
}

// collectRawChunks gathers rawChunks and identifies the file's first title.
func collectRawChunks(lines []string, actions []lineKind, headings map[int]headingInfo) (string, []rawChunk) {
	var firstTitle string
	var currentHeading string
	var currentLines []string
	var rawChunks []rawChunk
	inComment := false

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		switch actions[i] {
		case kindHeading:
			if hInfo, ok := headings[i]; ok {
				if hInfo.level == 1 && firstTitle == "" {
					firstTitle = hInfo.text
				}
				rawChunks = append(rawChunks, rawChunk{heading: currentHeading, lines: currentLines})
				currentHeading = hInfo.text
				currentLines = nil
			}
		case kindSetextUnderline:
			continue
		case kindVerbatim:
			currentLines = append(currentLines, line)
		case kindNormal:
			stripped, nextComment := stripHTMLComments(line, inComment)
			inComment = nextComment
			currentLines = append(currentLines, stripped)
		}
	}
	rawChunks = append(rawChunks, rawChunk{heading: currentHeading, lines: currentLines})
	return firstTitle, rawChunks
}

var gmParser = parser.NewParser(
	parser.WithBlockParsers(parser.DefaultBlockParsers()...),
)

// parseFileChunks parses content into chunks using goldmark CommonMark block parsing.
func parseFileChunks(repo, rev, filePath, content string, maxChunkBytes int) []Chunk {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	starts := lineOffsets(lines)
	src := []byte(normalized)
	doc := gmParser.Parse(text.NewReader(src))
	actions, headings := classifyLines(doc, src, starts, lines)
	firstTitle, rawChunks := collectRawChunks(lines, actions, headings)
	return buildKeptChunks(repo, rev, filePath, firstTitle, rawChunks, maxChunkBytes)
}

func buildKeptChunks(repo, rev, filePath, firstTitle string, rawChunks []rawChunk, maxBytes int) []Chunk {
	title := firstTitle
	if title == "" {
		title = filePath
	}
	var chunks []Chunk
	n := 0
	for _, rc := range rawChunks {
		body := strings.TrimSpace(strings.Join(rc.lines, "\n"))
		if body == "" {
			continue
		}
		parts := []string{body}
		if len(body) > maxBytes {
			parts = splitBody(body, maxBytes)
		}
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				continue
			}
			n++
			id := fmt.Sprintf("%s#%d", filePath, n)
			chunks = append(chunks, Chunk{
				ID:         id,
				Repository: repo,
				Revision:   rev,
				Path:       filePath,
				Title:      title,
				Heading:    rc.heading,
				Text:       trimmed,
			})
		}
	}
	return chunks
}
