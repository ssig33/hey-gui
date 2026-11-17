package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var errNotLoggedIn = errors.New("not logged in")

type cliError struct {
	code   int
	stderr string
}

func (e *cliError) Error() string {
	return T("hey exited with code %d: %s", e.code, e.stderr)
}

func errorText(err error) string {
	if errors.Is(err, errNotLoggedIn) {
		return T("Not logged in. Run `hey auth login` in Terminal.")
	}
	return err.Error()
}

// heyCLI runs the hey CLI as a child process and decodes its JSON output.
type heyCLI struct {
	path string
}

func (h heyCLI) run(args ...string) ([]byte, error) {
	cmd := exec.CommandContext(context.Background(), h.path, args...)
	cmd.Env = append(os.Environ(), "PATH="+strings.Join([]string{
		filepath.Dir(h.path), "/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin", os.Getenv("PATH"),
	}, ":"))
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if exitErr.ExitCode() == 3 {
			return nil, errNotLoggedIn
		}
		return nil, &cliError{code: exitErr.ExitCode(), stderr: strings.TrimSpace(stderr.String())}
	}
	if err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func decode[T any](h heyCLI, args ...string) (T, error) {
	data, err := h.run(args...)
	if err != nil {
		var zero T
		return zero, err
	}
	return decodeData[T](data)
}

func decodeData[T any](data []byte) (T, error) {
	var envelope struct {
		Data T `json:"data"`
	}
	err := json.Unmarshal(data, &envelope)
	return envelope.Data, err
}

func (h heyCLI) authenticated() (bool, error) {
	status, err := decode[struct {
		Authenticated bool `json:"authenticated"`
	}](h, "auth", "status", "--json")
	return status.Authenticated, err
}

// imboxData returns the raw JSON of the Imbox; decode it with parsePostings.
func (h heyCLI) imboxData(limit int) ([]byte, error) {
	args := []string{"box", "view", "imbox", "--json"}
	if limit > 0 {
		args = append(args, "--limit", strconv.Itoa(limit))
	} else {
		args = append(args, "--all")
	}
	return h.run(args...)
}

func parsePostings(data []byte) ([]posting, error) {
	box, err := decodeData[struct {
		Postings []posting `json:"postings"`
	}](data)
	return box.Postings, err
}

// threadData returns the raw `--json` and `--html` output of a thread. The HTML is
// fetched only for HTML emails.
func (h heyCLI) threadData(topicID int64) (jsonData, htmlData []byte, err error) {
	id := strconv.FormatInt(topicID, 10)
	jsonData, err = h.run("thread", "read", id, "--json")
	if err != nil {
		return nil, nil, err
	}
	entries, err := decodeData[[]entry](jsonData)
	if err != nil {
		return nil, nil, err
	}
	if hasHTMLEmail(entries) {
		htmlData, _ = h.run("thread", "read", id, "--html")
	}
	return jsonData, htmlData, nil
}

// parseThread decodes a thread and renders its bodies.
func parseThread(jsonData, htmlData []byte) ([]entry, error) {
	entries, err := decodeData[[]entry](jsonData)
	if err != nil {
		return nil, err
	}
	var bodies map[int64]string
	if htmlData != nil {
		bodies = entryHTML(htmlData)
	}
	for i := range entries {
		entries[i].HTML = bodies[entries[i].ID]
		entries[i].rendered = renderBody(&entries[i])
	}
	return entries, nil
}

func (h heyCLI) search(query string) ([]searchHit, error) {
	return decode[[]searchHit](h, "search", query, "--in", "imbox", "--json")
}

func (h heyCLI) markSeen(postingID int64) error {
	_, err := h.run("seen", strconv.FormatInt(postingID, 10), "--json")
	return err
}

// locateHey finds the hey binary. GUI apps don't inherit the shell's PATH.
func locateHey(override string) string {
	executable := func(p string) bool {
		st, err := os.Stat(p)
		return err == nil && !st.IsDir() && st.Mode()&0o111 != 0
	}
	home, _ := os.UserHomeDir()
	if override = strings.TrimSpace(override); override != "" {
		if strings.HasPrefix(override, "~/") {
			override = filepath.Join(home, override[2:])
		}
		if executable(override) {
			return override
		}
		return ""
	}
	if p := shellLookup(); p != "" && executable(p) {
		return p
	}
	candidates := []string{"/opt/homebrew/bin/hey", "/usr/local/bin/hey", filepath.Join(home, ".local/bin/hey")}
	miseDirs, _ := filepath.Glob(filepath.Join(home, ".local/share/mise/installs/*hey*/latest/hey"))
	candidates = append(candidates, miseDirs...)
	for _, c := range candidates {
		if executable(c) {
			return c
		}
	}
	return ""
}

// shellLookup runs `command -v hey` in an interactive login shell, where tools like mise are set up.
func shellLookup() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	cmd := exec.Command(shell, "-ilc", "command -v hey")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(lines[i], "/") {
			return strings.TrimSpace(lines[i])
		}
	}
	return ""
}

type contact struct {
	Name  string `json:"name"`
	Email string `json:"email_address"`
}

func (c *contact) display() string {
	switch {
	case c == nil:
		return "?"
	case c.Name != "" && c.Email != "":
		return c.Name + " <" + c.Email + ">"
	case c.Name != "":
		return c.Name
	case c.Email != "":
		return c.Email
	}
	return "?"
}

func joinContacts(cs []contact) string {
	parts := make([]string, len(cs))
	for i := range cs {
		parts[i] = cs[i].display()
	}
	return strings.Join(parts, ", ")
}

type posting struct {
	ID        int64    `json:"id"`
	TopicID   int64    `json:"topic_id"`
	Name      string   `json:"name"`
	Summary   string   `json:"summary"`
	Creator   *contact `json:"creator"`
	AltSender string   `json:"alternative_sender_name"`
	ActiveAt  string   `json:"active_at"`
	Count     int      `json:"visible_entry_count"`
	AppURL    string   `json:"app_url"`
	Seen      *bool    `json:"seen"`
}

type entry struct {
	ID         int64    `json:"id"`
	CreatedAt  string   `json:"created_at"`
	Creator    *contact `json:"creator"`
	AltSender  string   `json:"alternative_sender_name"`
	Kind       string   `json:"kind"`
	Recipients *struct {
		To []contact `json:"to"`
		Cc []contact `json:"cc"`
	} `json:"recipients"`
	Body   string `json:"body"`
	AppURL string `json:"app_url"`
	// HTML is the original HTML body, fetched only for HTML emails whose Markdown body
	// is just an attachment placeholder.
	HTML string `json:"-"`
	// rendered is the body converted for display, built off the UI goroutine.
	rendered *renderedBody
}

func (e *entry) sender() string {
	if e.AltSender != "" && (e.Creator == nil || e.AltSender != e.Creator.Name) {
		email := ""
		if e.Creator != nil {
			email = e.Creator.Email
		}
		return e.AltSender + " <" + email + ">"
	}
	return e.Creator.display()
}

// webURL opens this entry in the HEY web app.
func (e *entry) webURL() string {
	if e.AppURL == "" {
		return ""
	}
	return fmt.Sprintf("%s#__entry_%d", e.AppURL, e.ID)
}

type searchHit struct {
	TopicID   int64  `json:"topic_id"`
	Subject   string `json:"subject"`
	UpdatedAt string `json:"updated_at"`
	Messages  []struct {
		AltSender string   `json:"alternative_sender_name"`
		Creator   *contact `json:"creator"`
		AppURL    string   `json:"app_url"`
	} `json:"messages"`
}

// row is one line of the list, with display and filter values precomputed.
type row struct {
	topicID    int64
	postingID  int64 // 0 for search results; looked up from the list instead
	subject    string
	summary    string
	sender     string
	date       time.Time
	count      int
	appURL     string
	unread     bool
	searchText string
}

// version changes when the thread gets new entries; used to tell whether a cached thread is current.
func (r row) version() string {
	return fmt.Sprintf("%d-%d", r.date.Unix(), r.count)
}

func newRow(topicID, postingID int64, subject, summary, sender, email string, date time.Time, count int, appURL string, unread bool) row {
	if subject == "" {
		subject = T("(No subject)")
	}
	if count == 0 {
		count = 1
	}
	return row{
		topicID: topicID, postingID: postingID, subject: subject, summary: summary, sender: sender,
		date: date, count: count, appURL: appURL, unread: unread,
		searchText: strings.ToLower(strings.Join([]string{subject, summary, sender, email}, "\n")),
	}
}

func rowFromPosting(p posting) (row, bool) {
	if p.TopicID == 0 {
		return row{}, false
	}
	sender, email := p.AltSender, ""
	if p.Creator != nil {
		email = p.Creator.Email
		if sender == "" {
			sender = p.Creator.Name
		}
	}
	if sender == "" {
		sender = email
	}
	// Unread postings come back with seen = null.
	unread := p.Seen == nil || !*p.Seen
	return newRow(p.TopicID, p.ID, p.Name, p.Summary, sender, email, parseTime(p.ActiveAt), p.Count, p.AppURL, unread), true
}

func rowFromHit(h searchHit) row {
	sender, email, appURL := "", "", fmt.Sprintf("https://app.hey.com/topics/%d", h.TopicID)
	if len(h.Messages) > 0 {
		m := h.Messages[0]
		sender = m.AltSender
		if m.Creator != nil {
			email = m.Creator.Email
			if sender == "" {
				sender = m.Creator.Name
			}
		}
		if m.AppURL != "" {
			appURL = m.AppURL
		}
	}
	return newRow(h.TopicID, 0, h.Subject, "", sender, email, parseTime(h.UpdatedAt), len(h.Messages), appURL, false)
}

// parseTime reads RFC 3339 times, and the zone-less minute times of `hey thread read` as UTC.
func parseTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Local()
		}
	}
	return time.Time{}
}

func shortDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	if t.Year() == time.Now().Year() {
		return t.Format("01/02 15:04")
	}
	return t.Format("06/01/02")
}

func longDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return fmt.Sprintf("%s (%s) %s", t.Format("2006/01/02"), T(t.Format("Mon")), t.Format("15:04"))
}
