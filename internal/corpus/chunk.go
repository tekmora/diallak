package corpus

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type rawChunk struct {
	heading string
	lines   []string
}

// parseATXHeading parses a CommonMark ATX heading with 1 to 6 '#' characters.
// Setext headings (underlined with = or -) are unsupported.
func parseATXHeading(line string) (int, string, bool) {
	i := 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	rem := line[i:]
	hashes := 0
	for hashes < len(rem) && hashes < 7 && rem[hashes] == '#' {
		hashes++
	}
	if hashes < 1 || hashes > 6 {
		return 0, "", false
	}
	if len(rem) <= hashes || rem[hashes] != ' ' {
		return 0, "", false
	}
	raw := rem[hashes+1:]
	heading := strings.TrimRight(raw, " #\t\r\n")
	return hashes, heading, true
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

type fileParser struct {
	inFence        bool
	inComment      bool
	firstTitle     string
	currentHeading string
	currentLines   []string
	rawChunks      []rawChunk
}

func (p *fileParser) parseLine(line string) {
	trimmed := strings.TrimSpace(line)
	isFence := strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
	if p.inFence {
		if isFence {
			p.inFence = false
		}
		p.currentLines = append(p.currentLines, line)
		return
	}
	stripped, nextComment := stripHTMLComments(line, p.inComment)
	p.inComment = nextComment
	if !p.inComment && isFence {
		p.inFence = true
		p.currentLines = append(p.currentLines, line)
		return
	}
	if p.handleHeading(stripped) {
		return
	}
	p.currentLines = append(p.currentLines, stripped)
}

func (p *fileParser) handleHeading(stripped string) bool {
	if p.inComment {
		return false
	}
	level, heading, ok := parseATXHeading(stripped)
	if !ok {
		return false
	}
	if level == 1 && p.firstTitle == "" {
		p.firstTitle = heading
	}
	p.rawChunks = append(p.rawChunks, rawChunk{heading: p.currentHeading, lines: p.currentLines})
	p.currentHeading = heading
	p.currentLines = nil
	return true
}

func parseFileChunks(repo, rev, filePath, content string, maxChunkBytes int) []Chunk {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	p := &fileParser{}
	for _, line := range lines {
		p.parseLine(line)
	}
	p.rawChunks = append(p.rawChunks, rawChunk{heading: p.currentHeading, lines: p.currentLines})
	return buildKeptChunks(repo, rev, filePath, p.firstTitle, p.rawChunks, maxChunkBytes)
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
