package main

import (
	"regexp"
	"strings"
)

// A message body is rendered as a list of blocks, each holding styled spans.
// Both Markdown and HTML bodies are converted into this form.

type span struct {
	text   string
	bold   bool
	italic bool
	code   bool
	strike bool
	link   string
}

func (s span) sameStyle(o span) bool {
	return s.bold == o.bold && s.italic == o.italic && s.code == o.code && s.strike == o.strike && s.link == o.link
}

type blockKind int

const (
	blockPara blockKind = iota
	blockHeading
	blockCode
	blockRule
)

type block struct {
	kind   blockKind
	level  int    // heading level
	quote  int    // blockquote depth
	indent int    // list depth
	marker string // list marker of the first line, such as "•" or "1."
	spans  []span
}

type docBuilder struct {
	blocks        []block
	cur           *block
	quote, indent int
	marker        string
	style         span
}

func (b *docBuilder) start(kind blockKind, level int) {
	b.flush()
	b.cur = &block{kind: kind, level: level, quote: b.quote, indent: b.indent, marker: b.marker}
	b.marker = ""
}

func (b *docBuilder) flush() {
	if b.cur == nil {
		return
	}
	cur := b.cur
	b.cur = nil
	// Trim leading and trailing blank space of the block.
	for len(cur.spans) > 0 && strings.TrimSpace(cur.spans[0].text) == "" {
		cur.spans = cur.spans[1:]
	}
	for len(cur.spans) > 0 && strings.TrimSpace(cur.spans[len(cur.spans)-1].text) == "" {
		cur.spans = cur.spans[:len(cur.spans)-1]
	}
	if len(cur.spans) == 0 {
		if cur.marker != "" {
			b.marker = cur.marker
		}
		return
	}
	if cur.kind != blockCode {
		cur.spans[0].text = strings.TrimLeft(cur.spans[0].text, " \t\n")
		last := &cur.spans[len(cur.spans)-1]
		last.text = strings.TrimRight(last.text, " \t\n")
	}
	b.blocks = append(b.blocks, *cur)
}

func (b *docBuilder) rule() {
	b.flush()
	b.blocks = append(b.blocks, block{kind: blockRule, quote: b.quote, indent: b.indent})
}

func (b *docBuilder) text(s string) {
	if s == "" {
		return
	}
	if b.cur == nil {
		b.start(blockPara, 0)
	}
	sp := b.style
	sp.text = s
	if n := len(b.cur.spans); n > 0 && b.cur.spans[n-1].sameStyle(sp) {
		b.cur.spans[n-1].text += s
		return
	}
	b.cur.spans = append(b.cur.spans, sp)
}

// lastChar returns the last character written to the current block, or '\n' at its start.
func (b *docBuilder) lastChar() byte {
	if b.cur == nil {
		return '\n'
	}
	for i := len(b.cur.spans) - 1; i >= 0; i-- {
		if t := b.cur.spans[i].text; t != "" {
			return t[len(t)-1]
		}
	}
	return '\n'
}

func (b *docBuilder) finish() []block {
	b.flush()
	return b.blocks
}

var markdownHint = regexp.MustCompile("(?m)\\[[^\\]\\n]+\\]\\([^)\\s]+\\)|^#{1,6} |^ {0,3}[-*+] \\S|^ {0,3}\\d+[.)] \\S|^ {0,3}> |\\*\\*[^*\\n]+\\*\\*|^ {0,3}```|\\\\[\\\\`*_{}\\[\\]()#+\\-.!]")

// looksLikeMarkdown reports whether a body uses Markdown syntax and should be rendered as Markdown.
func looksLikeMarkdown(s string) bool {
	return markdownHint.MatchString(s)
}

// isAttachmentOnly reports whether the Markdown body is only an attachment placeholder,
// which is how HTML emails come back from `hey thread read`.
func isAttachmentOnly(body string) bool {
	body = strings.TrimSpace(body)
	if body == "" {
		return true
	}
	for _, line := range strings.Split(body, "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "📎") {
			return false
		}
	}
	return true
}
