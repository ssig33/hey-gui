package main

import "testing"

func TestUnescapeMarkdown(t *testing.T) {
	if got, want := unescapeMarkdown([]byte(`\*not bold\* &amp; &#65;&#x42;`)), "*not bold* & AB"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMarkdownBlocks(t *testing.T) {
	src := "## Subject\n\nline one\nline **two** [link](https://example.com)\n\n3. three\n4. four\n\n```\ncode\n```\n\n![](https://example.com/a.png)\n"
	bs := markdownBlocks(src)
	if got, want := blocksText(bs), "Subject\nline one\nline two link\nthree\nfour\ncode\n[image]"; got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
	if bs[0].kind != blockHeading || bs[0].level != 2 {
		t.Errorf("heading block = %+v", bs[0])
	}
	var bold, link bool
	for _, s := range bs[1].spans {
		bold = bold || s.bold && s.text == "two"
		link = link || s.link == "https://example.com" && s.text == "link"
	}
	if !bold || !link {
		t.Errorf("paragraph spans = %+v; want bold \"two\" and a link", bs[1].spans)
	}
	if bs[2].marker != "3." || bs[3].marker != "4." {
		t.Errorf("ordered list should keep its start number, got %q, %q", bs[2].marker, bs[3].marker)
	}
	if bs[4].kind != blockCode {
		t.Errorf("code block kind = %v", bs[4].kind)
	}
	if bs[5].spans[0].link != "https://example.com/a.png" {
		t.Errorf("image without alt should link to the image, got %+v", bs[5].spans)
	}
}

func TestMarkdownBlocksTable(t *testing.T) {
	bs := markdownBlocks("| a | b |\n|---|---|\n| 1 | 2 |\n")
	if got, want := blocksText(bs), "a  |  b\n1  |  2"; got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
	if !bs[0].spans[0].bold || bs[1].spans[0].bold {
		t.Errorf("only the header row should be bold: %+v", bs)
	}
}
