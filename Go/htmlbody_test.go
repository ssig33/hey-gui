package main

import (
	"strings"
	"testing"
)

func TestEntryHTMLExpandsTrixAttachments(t *testing.T) {
	doc := []byte(`<article data-entry-id="7"><header>meta</header>` +
		`<figure data-trix-attachment="{&quot;contentType&quot;:&quot;text/html&quot;,&quot;content&quot;:&quot;&lt;p&gt;Inner &lt;i&gt;mail&lt;/i&gt;&lt;/p&gt;&quot;}"></figure>` +
		`<shadow-content><template><p>From template</p></template></shadow-content>` +
		`</article>`)
	got := entryHTML(doc)[7]
	for _, want := range []string{"<p>Inner <i>mail</i></p>", "<p>From template</p>"} {
		if !strings.Contains(got, want) {
			t.Errorf("entryHTML = %q; missing %q", got, want)
		}
	}
	for _, unwanted := range []string{"meta", "<figure", "<shadow-content", "<template"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("entryHTML = %q; should not contain %q", got, unwanted)
		}
	}
}

func TestEntryHTMLActionTextImage(t *testing.T) {
	doc := []byte(`<article data-entry-id="3"><action-text-attachment content-type="image/png" url="https://example.com/a.png"></action-text-attachment></article>`)
	if got := entryHTML(doc)[3]; got != `<img src="https://example.com/a.png" alt=""/>` {
		t.Errorf("entryHTML = %q", got)
	}
}

func TestAttachmentHTML(t *testing.T) {
	tests := []struct {
		name, contentType, content, url, want string
	}{
		{"html content is used as is", "text/html; charset=utf-8", "<p>x</p>", "", "<p>x</p>"},
		{"image becomes img with escaped url", "image/jpeg", "", `https://e.com/a"b.jpg`, `<img src="https://e.com/a&#34;b.jpg" alt="">`},
		{"image without url is dropped", "image/png", "", "", ""},
		{"other types are dropped", "application/pdf", "", "https://e.com/a.pdf", ""},
	}
	for _, tt := range tests {
		if got := attachmentHTML(tt.contentType, tt.content, tt.url); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestHTMLBlocks(t *testing.T) {
	src := `<h3>Title</h3>
<div style="display: none">preheader</div>
<style>p { color: red }</style>
<p>Hello
   <a href="https://example.com">world</a> <a href="mailto:a@example.com">mail</a></p>
<ol><li>one</li><li>two</li></ol>
<pre>  keep   spaces</pre>`
	bs := htmlBlocks(src)
	if got, want := blocksText(bs), "Title\nHello world mail\none\ntwo\n  keep   spaces"; got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
	if bs[0].kind != blockHeading || bs[0].level != 3 {
		t.Errorf("first block = %+v, want heading level 3", bs[0])
	}
	var links []string
	for _, s := range bs[1].spans {
		if s.link != "" {
			links = append(links, s.link)
		}
	}
	if len(links) != 1 || links[0] != "https://example.com" {
		t.Errorf("links = %v; mailto links should not be clickable", links)
	}
	if bs[2].marker != "1." || bs[3].marker != "2." || bs[2].indent != 1 {
		t.Errorf("list blocks = %+v, %+v", bs[2], bs[3])
	}
	if bs[4].kind != blockCode {
		t.Errorf("pre block kind = %v, want code", bs[4].kind)
	}
}

func TestHTMLBlocksLargeDivIsHeading(t *testing.T) {
	bs := htmlBlocks(`<div style="font-size: 24px">Big</div><div style="font-size: 9px">small</div>`)
	if len(bs) != 2 || bs[0].kind != blockHeading || bs[1].kind != blockPara {
		t.Errorf("blocks = %+v", bs)
	}
}
