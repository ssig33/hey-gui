package main

import (
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	gtext "github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// unescapeMarkdown resolves backslash escapes and character references in raw text.
func unescapeMarkdown(raw []byte) string {
	return string(util.ResolveEntityNames(util.ResolveNumericReferences(util.UnescapePunctuations(raw))))
}

var markdownParser = goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser()

// markdownBlocks converts a Markdown body into blocks. Soft line breaks are kept as
// line breaks, since mail text is laid out line by line.
func markdownBlocks(src string) []block {
	source := []byte(src)
	doc := markdownParser.Parse(gtext.NewReader(source))
	b := &docBuilder{}
	mdBlock(b, doc, source)
	return b.finish()
}

func mdBlock(b *docBuilder, n ast.Node, src []byte) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch c := c.(type) {
		case *ast.Paragraph, *ast.TextBlock:
			b.start(blockPara, 0)
			mdInline(b, c, src)
			b.flush()
		case *ast.Heading:
			b.start(blockHeading, c.Level)
			mdInline(b, c, src)
			b.flush()
		case *ast.Blockquote:
			b.flush()
			b.quote++
			mdBlock(b, c, src)
			b.flush()
			b.quote--
		case *ast.List:
			b.flush()
			b.indent++
			i := c.Start
			for item := c.FirstChild(); item != nil; item = item.NextSibling() {
				if c.IsOrdered() {
					b.marker = fmt.Sprintf("%d.", i)
					i++
				} else {
					b.marker = "•"
				}
				mdBlock(b, item, src)
				b.flush()
				b.marker = ""
			}
			b.indent--
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			b.start(blockCode, 0)
			lines := c.Lines()
			var sb strings.Builder
			for i := 0; i < lines.Len(); i++ {
				seg := lines.At(i)
				sb.Write(seg.Value(src))
			}
			b.text(strings.TrimRight(sb.String(), "\n"))
			b.flush()
		case *ast.ThematicBreak:
			b.rule()
		case *ast.HTMLBlock:
			b.start(blockPara, 0)
			lines := c.Lines()
			for i := 0; i < lines.Len(); i++ {
				seg := lines.At(i)
				b.text(string(seg.Value(src)))
			}
			b.flush()
		case *east.Table:
			b.flush()
			for row := c.FirstChild(); row != nil; row = row.NextSibling() {
				b.start(blockPara, 0)
				if _, header := row.(*east.TableHeader); header {
					b.style.bold = true
				}
				for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
					if cell != row.FirstChild() {
						b.text("  |  ")
					}
					mdInline(b, cell, src)
				}
				b.style.bold = false
				b.flush()
			}
		default:
			mdBlock(b, c, src)
		}
	}
}

func mdInline(b *docBuilder, n ast.Node, src []byte) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		saved := b.style
		switch c := c.(type) {
		case *ast.Text:
			b.text(unescapeMarkdown(c.Segment.Value(src)))
			if c.SoftLineBreak() || c.HardLineBreak() {
				b.text("\n")
			}
		case *ast.String:
			b.text(string(c.Value))
		case *ast.CodeSpan:
			b.style.code = true
			mdInline(b, c, src)
		case *ast.Emphasis:
			if c.Level >= 2 {
				b.style.bold = true
			} else {
				b.style.italic = true
			}
			mdInline(b, c, src)
		case *east.Strikethrough:
			b.style.strike = true
			mdInline(b, c, src)
		case *ast.Link:
			b.style.link = string(c.Destination)
			mdInline(b, c, src)
		case *ast.AutoLink:
			b.style.link = string(c.URL(src))
			b.text(string(c.Label(src)))
		case *ast.Image:
			// Show the alt text as a link to the image.
			b.style.link = string(c.Destination)
			before := b.lastChar()
			mdInline(b, c, src)
			if b.lastChar() == before {
				b.text("[image]")
			}
		case *ast.RawHTML:
			for i := 0; i < c.Segments.Len(); i++ {
				seg := c.Segments.At(i)
				if raw := strings.ToLower(string(seg.Value(src))); strings.HasPrefix(raw, "<br") {
					b.text("\n")
				}
			}
		default:
			mdInline(b, c, src)
		}
		b.style = saved
	}
}
