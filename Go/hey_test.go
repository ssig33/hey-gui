package main

import (
	"strings"
	"testing"
)

func TestParsePostings(t *testing.T) {
	data := []byte(`{"ok":true,"data":{"postings":[
		{"id":11,"topic_id":101,"name":"Hello","seen":true,"visible_entry_count":2,
		 "creator":{"name":"Alice","email_address":"alice@example.com"}},
		{"id":12,"topic_id":102,"name":"Unread","seen":null}
	]}}`)
	ps, err := parsePostings(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 {
		t.Fatalf("got %d postings, want 2", len(ps))
	}
	if ps[0].TopicID != 101 || ps[0].Creator == nil || ps[0].Creator.Email != "alice@example.com" {
		t.Errorf("first posting decoded wrong: %+v", ps[0])
	}
	if ps[0].Seen == nil || !*ps[0].Seen {
		t.Errorf("seen=true should decode as true, got %v", ps[0].Seen)
	}
	if ps[1].Seen != nil {
		t.Errorf("seen=null should stay nil so the posting counts as unread, got %v", *ps[1].Seen)
	}
}

func TestParsePostingsRejectsBrokenJSON(t *testing.T) {
	if _, err := parsePostings([]byte(`{"data":`)); err == nil {
		t.Fatal("expected an error for truncated JSON")
	}
}

func TestDecodeDataWithoutDataKey(t *testing.T) {
	got, err := decodeData[[]entry]([]byte(`{"ok":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %d entries, want none", len(got))
	}
}

func TestParseThreadAttachesHTMLToItsEntry(t *testing.T) {
	jsonData := []byte(`{"data":[
		{"id":1,"body":"📎 message.html"},
		{"id":2,"body":"plain reply"}
	]}`)
	htmlData := []byte(`<html><body>
		<article data-entry-id="1"><header>From Alice</header><p>Hi <b>there</b></p></article>
		<article data-entry-id="2"><header>From Bob</header><p>plain reply</p></article>
	</body></html>`)

	entries, err := parseThread(jsonData, htmlData)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if !strings.Contains(entries[0].HTML, "<b>there</b>") || strings.Contains(entries[0].HTML, "From Alice") {
		t.Errorf("entry 1 HTML = %q; want the body without the header", entries[0].HTML)
	}
	if r := entries[0].rendered; r == nil || r.plain || blocksText(r.blocks) != "Hi there" {
		t.Errorf("attachment-only entry should render from its HTML, got %+v", r)
	}
	// A body that is not just an attachment placeholder is shown as is, even when HTML exists.
	if r := entries[1].rendered; r == nil || !r.plain || r.text != "plain reply" {
		t.Errorf("entry 2 should render as plain text, got %+v", r)
	}
}

func TestParseThreadWithoutHTML(t *testing.T) {
	entries, err := parseThread([]byte(`{"data":[{"id":1,"body":"# Title\n\ntext"}]}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].HTML != "" {
		t.Errorf("HTML = %q, want empty", entries[0].HTML)
	}
	r := entries[0].rendered
	if r == nil || r.plain || len(r.blocks) != 2 || r.blocks[0].kind != blockHeading {
		t.Errorf("Markdown body should render as heading + paragraph, got %+v", r)
	}
}

// blocksText joins the text of blocks, one block per line.
func blocksText(bs []block) string {
	lines := make([]string, len(bs))
	for i, b := range bs {
		var sb strings.Builder
		for _, s := range b.spans {
			sb.WriteString(s.text)
		}
		lines[i] = sb.String()
	}
	return strings.Join(lines, "\n")
}
