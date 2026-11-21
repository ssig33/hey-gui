package main

import (
	"testing"
	"time"
)

func TestEntrySender(t *testing.T) {
	tests := []struct {
		name string
		e    entry
		want string
	}{
		{"creator only", entry{Creator: &contact{Name: "Alice", Email: "a@example.com"}}, "Alice <a@example.com>"},
		{"alt sender with creator email", entry{AltSender: "Team", Creator: &contact{Name: "Alice", Email: "a@example.com"}}, "Team <a@example.com>"},
		{"alt sender equal to creator name", entry{AltSender: "Alice", Creator: &contact{Name: "Alice", Email: "a@example.com"}}, "Alice <a@example.com>"},
		{"alt sender without creator", entry{AltSender: "Team"}, "Team"},
		{"alt sender with empty creator email", entry{AltSender: "Team", Creator: &contact{Name: "Alice"}}, "Team"},
		{"nothing known", entry{}, "?"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.e.sender(); got != tt.want {
				t.Errorf("sender() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRowFromPostingSenderAndUnread(t *testing.T) {
	yes, no := true, false
	tests := []struct {
		name       string
		p          posting
		wantSender string
		wantUnread bool
	}{
		{"alt sender wins", posting{TopicID: 1, AltSender: "Team", Creator: &contact{Name: "Alice", Email: "a@example.com"}, Seen: &yes}, "Team", false},
		{"creator name", posting{TopicID: 1, Creator: &contact{Name: "Alice", Email: "a@example.com"}, Seen: &yes}, "Alice", false},
		{"email when no name", posting{TopicID: 1, Creator: &contact{Email: "a@example.com"}, Seen: &no}, "a@example.com", true},
		{"seen null is unread", posting{TopicID: 1, Creator: &contact{Name: "Alice"}}, "Alice", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, ok := rowFromPosting(tt.p)
			if !ok {
				t.Fatal("rowFromPosting rejected a posting with a topic ID")
			}
			if r.sender != tt.wantSender || r.unread != tt.wantUnread {
				t.Errorf("got sender %q unread %v, want %q %v", r.sender, r.unread, tt.wantSender, tt.wantUnread)
			}
		})
	}
}

func TestRowFromPostingWithoutTopicIsSkipped(t *testing.T) {
	if _, ok := rowFromPosting(posting{ID: 5}); ok {
		t.Error("a posting without a topic ID should be skipped")
	}
}

func TestNewRowDefaults(t *testing.T) {
	r := newRow(1, 0, "", "", "Alice", "", time.Time{}, 0, "", false)
	if r.subject != T("(No subject)") {
		t.Errorf("empty subject became %q", r.subject)
	}
	if r.count != 1 {
		t.Errorf("count 0 became %d, want 1", r.count)
	}
}

func TestRowFromHit(t *testing.T) {
	hit := searchHit{TopicID: 42, Subject: "Hi"}
	hit.Messages = append(hit.Messages, struct {
		AltSender string   `json:"alternative_sender_name"`
		Creator   *contact `json:"creator"`
		AppURL    string   `json:"app_url"`
	}{Creator: &contact{Email: "a@example.com"}})

	r := rowFromHit(hit)
	if r.sender != "a@example.com" {
		t.Errorf("sender = %q, want the email when the name is empty", r.sender)
	}
	if r.appURL != "https://app.hey.com/topics/42" {
		t.Errorf("appURL = %q, want the topic URL when the message has none", r.appURL)
	}
	if r.postingID != 0 || r.unread {
		t.Errorf("search rows should have no posting ID and not be unread: %+v", r)
	}
}

func TestParseTime(t *testing.T) {
	tests := []struct {
		in   string
		want time.Time
	}{
		{"2026-11-17T02:14:05Z", time.Date(2026, 11, 17, 2, 14, 5, 0, time.UTC)},
		{"2026-11-17T11:14:05.123+09:00", time.Date(2026, 11, 17, 2, 14, 5, 123000000, time.UTC)},
		{"2026-11-17T02:14", time.Date(2026, 11, 17, 2, 14, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		if got := parseTime(tt.in); !got.Equal(tt.want) {
			t.Errorf("parseTime(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
	for _, in := range []string{"", "yesterday"} {
		if got := parseTime(in); !got.IsZero() {
			t.Errorf("parseTime(%q) = %v, want zero", in, got)
		}
	}
}
