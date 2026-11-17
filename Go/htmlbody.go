package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// entryHTML splits the output of `hey thread read --html` into the body of each entry.
// HTML emails are stored as Trix attachments whose content is escaped HTML wrapped in
// <shadow-content><template>; expandHTML turns them back into plain HTML.
func entryHTML(doc []byte) map[int64]string {
	root, err := html.Parse(bytes.NewReader(doc))
	if err != nil {
		return nil
	}
	result := map[int64]string{}
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.DataAtom == atom.Article {
			id, _ := strconv.ParseInt(attr(n, "data-entry-id"), 10, 64)
			var buf bytes.Buffer
			expandHTML(n)
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && c.DataAtom == atom.Header {
					continue
				}
				_ = html.Render(&buf, c)
			}
			result[id] = buf.String()
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return result
}

// expandHTML rewrites Trix attachments and template wrappers in place.
func expandHTML(n *html.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		if c.Type == html.ElementNode {
			switch {
			case c.DataAtom == atom.Figure && attr(c, "data-trix-attachment") != "":
				var a struct {
					ContentType string `json:"contentType"`
					Content     string `json:"content"`
					URL         string `json:"url"`
				}
				_ = json.Unmarshal([]byte(attr(c, "data-trix-attachment")), &a)
				replaceWithFragment(n, c, attachmentHTML(a.ContentType, a.Content, a.URL))
			case c.Data == "action-text-attachment":
				replaceWithFragment(n, c, attachmentHTML(attr(c, "content-type"), attr(c, "content"), attr(c, "url")))
			case c.Data == "shadow-content" || c.DataAtom == atom.Template:
				expandHTML(c)
				for gc := c.FirstChild; gc != nil; {
					gnext := gc.NextSibling
					c.RemoveChild(gc)
					n.InsertBefore(gc, c)
					gc = gnext
				}
				n.RemoveChild(c)
			default:
				expandHTML(c)
			}
		}
		c = next
	}
}

func attachmentHTML(contentType, content, url string) string {
	switch {
	case strings.HasPrefix(contentType, "text/html"):
		return content
	case strings.HasPrefix(contentType, "image") && url != "":
		return fmt.Sprintf(`<img src="%s" alt="">`, html.EscapeString(url))
	}
	return ""
}

func replaceWithFragment(parent, old *html.Node, fragment string) {
	nodes, _ := html.ParseFragment(strings.NewReader(fragment), &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div})
	for _, nn := range nodes {
		wrapper := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
		wrapper.AppendChild(nn)
		expandHTML(wrapper)
		for c := wrapper.FirstChild; c != nil; {
			next := c.NextSibling
			wrapper.RemoveChild(c)
			parent.InsertBefore(c, old)
			c = next
		}
	}
	parent.RemoveChild(old)
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

var (
	spaceRun   = regexp.MustCompile(`\s+`)
	hiddenRule = regexp.MustCompile(`display:\s*none|visibility:\s*hidden|max-height:\s*0|font-size:\s*0`)
	boldRule   = regexp.MustCompile(`font-weight:\s*(bold|[6-9]00)`)
	sizeRule   = regexp.MustCompile(`font-size:\s*(\d+)px`)
)

// htmlBlocks converts an HTML body into blocks.
func htmlBlocks(src string) []block {
	nodes, err := html.ParseFragment(strings.NewReader(src), &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body})
	if err != nil {
		return nil
	}
	b := &docBuilder{}
	for _, n := range nodes {
		htmlNode(b, n, false)
	}
	return b.finish()
}

func htmlNode(b *docBuilder, n *html.Node, pre bool) {
	switch n.Type {
	case html.TextNode:
		t := n.Data
		if !pre {
			t = spaceRun.ReplaceAllString(t, " ")
			if last := b.lastChar(); last == ' ' || last == '\n' {
				t = strings.TrimLeft(t, " ")
			}
		}
		b.text(t)
		return
	case html.ElementNode:
	default:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			htmlNode(b, c, pre)
		}
		return
	}
	style := strings.ToLower(attr(n, "style"))
	if hiddenRule.MatchString(style) {
		return
	}
	saved := b.style
	if boldRule.MatchString(style) {
		b.style.bold = true
	}
	children := func() {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			htmlNode(b, c, pre)
		}
	}
	switch n.DataAtom {
	case atom.Script, atom.Style, atom.Head, atom.Title, atom.Meta, atom.Link:
		return
	case atom.Br:
		b.text("\n")
	case atom.Hr:
		b.rule()
	case atom.Img:
		if alt := strings.TrimSpace(attr(n, "alt")); alt != "" {
			b.text(alt)
		}
	case atom.A:
		if href := attr(n, "href"); href != "" && !strings.HasPrefix(href, "mailto:") {
			b.style.link = href
		}
		children()
	case atom.B, atom.Strong:
		b.style.bold = true
		children()
	case atom.I, atom.Em:
		b.style.italic = true
		children()
	case atom.S, atom.Strike, atom.Del:
		b.style.strike = true
		children()
	case atom.Code, atom.Tt, atom.Kbd:
		b.style.code = true
		children()
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		b.start(blockHeading, int(n.Data[1]-'0'))
		children()
		b.flush()
	case atom.Pre:
		b.start(blockCode, 0)
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			htmlNode(b, c, true)
		}
		b.flush()
	case atom.Blockquote:
		b.flush()
		b.quote++
		children()
		b.flush()
		b.quote--
	case atom.Ul, atom.Ol:
		b.flush()
		b.indent++
		i := 1
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.DataAtom == atom.Li {
				b.flush()
				if n.DataAtom == atom.Ol {
					b.marker = fmt.Sprintf("%d.", i)
					i++
				} else {
					b.marker = "•"
				}
			}
			htmlNode(b, c, pre)
		}
		b.flush()
		b.marker = ""
		b.indent--
	case atom.Td, atom.Th:
		if last := b.lastChar(); last != ' ' && last != '\n' {
			b.text(" ")
		}
		if n.DataAtom == atom.Th {
			b.style.bold = true
		}
		children()
	case atom.Div, atom.P:
		// Emails often style headings as large divs.
		b.flush()
		if m := sizeRule.FindStringSubmatch(style); m != nil && len(m[1]) >= 2 && m[1] >= "20" {
			b.start(blockHeading, 2)
		}
		children()
		b.flush()
	case atom.Section, atom.Article, atom.Header, atom.Footer, atom.Main, atom.Nav, atom.Aside,
		atom.Table, atom.Tbody, atom.Thead, atom.Tfoot, atom.Tr, atom.Li, atom.Dl, atom.Dt, atom.Dd, atom.Figure, atom.Figcaption,
		atom.Center, atom.Address:
		b.flush()
		children()
		b.flush()
	default:
		children()
	}
	b.style = saved
}

// openHTML writes an entry's HTML to a temporary file and opens it in the default browser.
func openHTML(entryID int64, body string) {
	dir := filepath.Join(os.TempDir(), "hey-gui")
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, fmt.Sprintf("entry-%d.html", entryID))
	page := "<!doctype html><html><head><meta charset=\"utf-8\"></head><body>" + body + "</body></html>"
	if os.WriteFile(path, []byte(page), 0o644) == nil {
		openURL("file://" + path)
	}
}
