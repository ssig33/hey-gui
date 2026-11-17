package main

import (
	"os/exec"
	"sort"
	"strings"
	"time"
)

type authState int

const (
	authChecking authState = iota
	authLoggedIn
	authLoggedOut
	authCLIMissing
	authError
)

// model holds the app state. It is only touched on the UI goroutine; background
// work hands results back through post.
type model struct {
	post func(func())
	cfg  *config
	cli  heyCLI

	auth        authState
	authErr     string
	loading     bool
	loadingRest bool
	loadErr     string
	generation  int

	rows          []row
	serverResults []row
	hasServer     bool
	searching     bool
	query         string
	unread        map[int64]bool
	postingIDs    map[int64]int64
	visible       []row
	selected      int64

	threads    map[int64][]entry
	threadErrs map[int64]string
	inFlight   map[int64]bool
}

func newModel(cfg *config, post func(func())) *model {
	return &model{cfg: cfg, post: post, unread: map[int64]bool{}, postingIDs: map[int64]int64{},
		threads: map[int64][]entry{}, threadErrs: map[int64]string{}, inFlight: map[int64]bool{}}
}

func (m *model) reload() {
	m.generation++
	gen := m.generation
	m.auth = authChecking
	m.loadErr = ""
	m.threads = map[int64][]entry{}
	m.threadErrs = map[int64]string{}
	m.inFlight = map[int64]bool{}
	override := m.cfg.HeyPath
	go func() {
		path := locateHey(override)
		if path == "" {
			m.post(func() { m.auth = authCLIMissing })
			return
		}
		cli := heyCLI{path: path}
		ok, err := cli.authenticated()
		m.post(func() {
			if gen != m.generation {
				return
			}
			m.cli = cli
			switch {
			case err == errNotLoggedIn || (err == nil && !ok):
				m.auth = authLoggedOut
			case err != nil:
				m.auth = authError
				m.authErr = errorText(err)
			default:
				m.auth = authLoggedIn
				m.loading = true
				go m.loadImbox(cli, gen)
			}
		})
	}()
}

// loadImbox shows the first 100 postings, then the rest. --all takes about 20 seconds for 1000 postings.
func (m *model) loadImbox(cli heyCLI, gen int) {
	first, err := cli.imbox(100)
	m.post(func() {
		if gen != m.generation {
			return
		}
		m.loading = false
		if err != nil {
			m.loadErr = errorText(err)
			return
		}
		m.setRows(first)
		m.loadingRest = true
	})
	if err != nil {
		return
	}
	all, err := cli.imbox(0)
	m.post(func() {
		if gen != m.generation {
			return
		}
		m.loadingRest = false
		if err != nil {
			m.loadErr = errorText(err)
			return
		}
		m.setRows(all)
	})
}

func (m *model) setRows(postings []posting) {
	seen := map[int64]bool{}
	m.rows = m.rows[:0:0]
	m.unread = map[int64]bool{}
	m.postingIDs = map[int64]int64{}
	for _, p := range postings {
		r, ok := rowFromPosting(p)
		if !ok || seen[r.topicID] {
			continue
		}
		seen[r.topicID] = true
		m.rows = append(m.rows, r)
		m.postingIDs[r.topicID] = r.postingID
		if r.unread {
			m.unread[r.topicID] = true
		}
	}
	m.refilter()
	if m.indexOf(m.selected) < 0 && len(m.visible) > 0 {
		m.selected = m.visible[0].topicID
		m.selectionChanged()
	}
}

func (m *model) refilter() {
	var list []row
	if m.hasServer {
		list = append(list, m.serverResults...)
	} else {
		q := strings.ToLower(strings.TrimSpace(m.query))
		for _, r := range m.rows {
			if q == "" || strings.Contains(r.searchText, q) {
				list = append(list, r)
			}
		}
	}
	if m.cfg.UnreadOnly {
		filtered := list[:0]
		for _, r := range list {
			if m.unread[r.topicID] {
				filtered = append(filtered, r)
			}
		}
		list = filtered
	}
	newest := m.cfg.NewestFirst
	sort.SliceStable(list, func(i, j int) bool {
		if newest {
			return list[i].date.After(list[j].date)
		}
		return list[i].date.Before(list[j].date)
	})
	m.visible = list
}

func (m *model) indexOf(topicID int64) int {
	for i, r := range m.visible {
		if r.topicID == topicID {
			return i
		}
	}
	return -1
}

func (m *model) selectedRow() (row, bool) {
	if i := m.indexOf(m.selected); i >= 0 {
		return m.visible[i], true
	}
	return row{}, false
}

// fetchThread loads a thread once; concurrent requests for the same topic share one call.
func (m *model) fetchThread(topicID int64, prefetch bool) {
	if _, ok := m.threads[topicID]; ok || m.inFlight[topicID] {
		return
	}
	m.inFlight[topicID] = true
	cli := m.cli
	go func() {
		entries, err := cli.thread(topicID)
		if err == nil && hasHTMLEmail(entries) {
			if bodies, herr := cli.threadHTML(topicID); herr == nil {
				for i := range entries {
					entries[i].HTML = bodies[entries[i].ID]
				}
			}
		}
		m.post(func() {
			delete(m.inFlight, topicID)
			if err != nil {
				m.threadErrs[topicID] = errorText(err)
				return
			}
			m.threads[topicID] = entries
			if !prefetch {
				m.prefetchNeighbors(topicID)
			}
		})
	}()
}

// prefetchNeighbors loads the threads before and after the given one.
func (m *model) prefetchNeighbors(topicID int64) {
	i := m.indexOf(topicID)
	for _, j := range []int{i + 1, i - 1} {
		if i >= 0 && j >= 0 && j < len(m.visible) {
			m.fetchThread(m.visible[j].topicID, true)
		}
	}
}

// selectionChanged waits briefly so holding j doesn't fetch every thread along the way.
func (m *model) selectionChanged() {
	id := m.selected
	if _, ok := m.threads[id]; ok {
		m.prefetchNeighbors(id)
		return
	}
	time.AfterFunc(80*time.Millisecond, func() {
		m.post(func() {
			if m.selected == id {
				m.fetchThread(id, false)
			}
		})
	})
}

func (m *model) setQuery(q string) {
	if q == m.query {
		return
	}
	m.query = q
	m.refilter()
}

func (m *model) searchServer() {
	q := strings.TrimSpace(m.query)
	if q == "" || m.cli.path == "" {
		return
	}
	m.searching = true
	cli := m.cli
	go func() {
		hits, err := cli.search(q)
		m.post(func() {
			m.searching = false
			if strings.TrimSpace(m.query) != q {
				return
			}
			if err != nil {
				m.loadErr = errorText(err)
				return
			}
			seen := map[int64]bool{}
			m.serverResults = nil
			for _, h := range hits {
				if !seen[h.TopicID] {
					seen[h.TopicID] = true
					m.serverResults = append(m.serverResults, rowFromHit(h))
				}
			}
			m.hasServer = true
			m.refilter()
			if len(m.visible) > 0 {
				m.selected = m.visible[0].topicID
				m.selectionChanged()
			}
		})
	}()
}

func (m *model) clearSearch() {
	m.query = ""
	m.hasServer = false
	m.serverResults = nil
	m.refilter()
}

func (m *model) move(delta int) {
	if len(m.visible) == 0 {
		return
	}
	i := m.indexOf(m.selected)
	if i < 0 {
		if delta > 0 {
			i = -1
		} else {
			i = len(m.visible)
		}
	}
	i = min(max(i+delta, 0), len(m.visible)-1)
	m.selectIndex(i)
}

func (m *model) selectIndex(i int) {
	if i < 0 || i >= len(m.visible) || m.visible[i].topicID == m.selected {
		return
	}
	m.selected = m.visible[i].topicID
	m.selectionChanged()
}

func (m *model) toggleSort() {
	m.cfg.NewestFirst = !m.cfg.NewestFirst
	m.cfg.save()
	m.refilter()
}

func (m *model) toggleUnreadOnly() {
	m.cfg.UnreadOnly = !m.cfg.UnreadOnly
	m.cfg.save()
	m.refilter()
}

// markSeenAndAdvance marks the selected thread as seen and moves to the next one.
func (m *model) markSeenAndAdvance() {
	i := m.indexOf(m.selected)
	if i < 0 {
		return
	}
	r := m.visible[i]
	var next int64
	if i+1 < len(m.visible) {
		next = m.visible[i+1].topicID
	} else if i > 0 {
		next = m.visible[i-1].topicID
	}
	postingID := r.postingID
	if postingID == 0 {
		postingID = m.postingIDs[r.topicID]
	}
	if m.unread[r.topicID] && postingID != 0 {
		delete(m.unread, r.topicID)
		m.refilter()
		cli := m.cli
		go func() {
			if err := cli.markSeen(postingID); err != nil {
				m.post(func() {
					m.unread[r.topicID] = true
					m.refilter()
					m.loadErr = T("Couldn't mark as seen: %s", errorText(err))
				})
			}
		}()
	}
	if next != 0 {
		m.selected = next
		m.selectionChanged()
	}
}

func openURL(url string) {
	if url != "" {
		_ = exec.Command("open", url).Start()
	}
}

func hasHTMLEmail(entries []entry) bool {
	for _, e := range entries {
		if isAttachmentOnly(e.Body) {
			return true
		}
	}
	return false
}
