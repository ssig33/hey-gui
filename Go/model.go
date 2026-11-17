package main

import (
	"os/exec"
	"runtime"
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

	// threads holds loaded threads, tagged with the list version they were fetched for.
	threads    map[int64]threadSnapshot
	threadErrs map[int64]string
	inFlight   map[int64]bool
	// versions of each thread in the list: change when a thread gets new entries.
	versions map[int64]string
	// shown is the thread whose body the thread pane shows. It follows the selection
	// once the selection settles, so holding j doesn't lay out every thread.
	shown int64

	prefetchQueue   []int64
	prefetchRunning int
}

type threadSnapshot struct {
	version string
	entries []entry
}

const prefetchLimit = 3

func newModel(cfg *config, post func(func())) *model {
	return &model{cfg: cfg, post: post, unread: map[int64]bool{}, postingIDs: map[int64]int64{},
		threads: map[int64]threadSnapshot{}, threadErrs: map[int64]string{}, inFlight: map[int64]bool{},
		versions: map[int64]string{}}
}

func (m *model) reload() {
	m.generation++
	gen := m.generation
	if len(m.rows) == 0 {
		m.auth = authChecking
	}
	m.loadErr = ""
	m.threadErrs = map[int64]string{}
	m.prefetchQueue = nil
	override := m.cfg.HeyPath
	showCached := len(m.rows) == 0
	go func() {
		var cached []posting
		if showCached {
			// Show the list from the last launch while checking login and fetching the latest one.
			if data := loadCachedImbox(); data != nil {
				if postings, err := parsePostings(data); err == nil && len(postings) > 0 {
					cached = postings
					m.post(func() {
						if gen == m.generation && len(m.rows) == 0 {
							m.auth = authLoggedIn
							m.setRows(postings)
						}
					})
				}
			}
		}
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
				m.loading = len(m.rows) == 0
				if m.selected != 0 {
					m.selectionChanged()
				}
				go m.loadImbox(cli, gen, cached)
			}
		})
	}()
}

// loadImbox shows the first 100 postings, then the rest. --all takes about 20 seconds for 1000 postings.
// Until the rest arrives, cached postings older than the first 100 stay in the list.
func (m *model) loadImbox(cli heyCLI, gen int, cached []posting) {
	data, err := cli.imboxData(100)
	var first []posting
	if err == nil {
		first, err = parsePostings(data)
	}
	m.post(func() {
		if gen != m.generation {
			return
		}
		m.loading = false
		if err != nil {
			m.loadErr = errorText(err)
			return
		}
		ids := map[int64]bool{}
		for _, p := range first {
			ids[p.TopicID] = true
		}
		merged := first
		for _, p := range cached {
			if !ids[p.TopicID] {
				merged = append(merged, p)
			}
		}
		m.setRows(merged)
		m.prefetchList()
		m.loadingRest = true
	})
	if err != nil {
		return
	}
	data, err = cli.imboxData(0)
	var all []posting
	if err == nil {
		all, err = parsePostings(data)
	}
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
		m.prefetchList()
		keep := map[int64]bool{}
		for _, r := range m.rows {
			keep[r.topicID] = true
		}
		go func() {
			saveCachedImbox(data)
			pruneCachedThreads(keep)
		}()
	})
}

func (m *model) setRows(postings []posting) {
	seen := map[int64]bool{}
	m.rows = m.rows[:0:0]
	m.unread = map[int64]bool{}
	m.postingIDs = map[int64]int64{}
	m.versions = map[int64]string{}
	for _, p := range postings {
		r, ok := rowFromPosting(p)
		if !ok || seen[r.topicID] {
			continue
		}
		seen[r.topicID] = true
		m.rows = append(m.rows, r)
		m.postingIDs[r.topicID] = r.postingID
		m.versions[r.topicID] = r.version()
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

// isFresh reports whether the loaded thread matches the current list.
func (m *model) isFresh(topicID int64) bool {
	t, ok := m.threads[topicID]
	return ok && t.version != "" && t.version == m.versions[topicID]
}

// fetchThread loads the latest version of a thread: from the disk cache if it matches the
// current list, otherwise from hey. Concurrent requests for the same topic share one call.
func (m *model) fetchThread(topicID int64, prefetch bool) {
	if m.isFresh(topicID) || m.inFlight[topicID] || m.cli.path == "" {
		if prefetch {
			m.prefetchDone()
		}
		return
	}
	m.inFlight[topicID] = true
	cli, version := m.cli, m.versions[topicID]
	go func() {
		var entries []entry
		var err error
		if t, ok := loadCachedThread(topicID); ok && version != "" && t.Version == version {
			entries, err = parseThread(t.JSON, t.HTML)
		} else {
			var jsonData, htmlData []byte
			jsonData, htmlData, err = cli.threadData(topicID)
			if err == nil {
				entries, err = parseThread(jsonData, htmlData)
			}
			if err == nil {
				saveCachedThread(topicID, cachedThread{Version: version, JSON: jsonData, HTML: htmlData})
			}
		}
		m.post(func() {
			delete(m.inFlight, topicID)
			if err != nil {
				m.threadErrs[topicID] = errorText(err)
			} else {
				delete(m.threadErrs, topicID)
				m.threads[topicID] = threadSnapshot{version, entries}
			}
			if prefetch {
				m.prefetchDone()
			}
		})
	}()
}

// prefetchNeighbors loads the threads around the given one ahead of the rest of the queue.
func (m *model) prefetchNeighbors(topicID int64) {
	i := m.indexOf(topicID)
	if i < 0 {
		return
	}
	var ids []int64
	for _, d := range []int{1, 2, 3, 4, 5, -1, -2} {
		if j := i + d; j >= 0 && j < len(m.visible) {
			ids = append(ids, m.visible[j].topicID)
		}
	}
	m.enqueuePrefetch(ids, true)
}

// prefetchList loads unread threads and the top of the list in the background.
func (m *model) prefetchList() {
	var ids []int64
	for i, r := range m.visible {
		if i < 30 || m.unread[r.topicID] {
			ids = append(ids, r.topicID)
		}
	}
	m.enqueuePrefetch(ids, false)
}

func (m *model) enqueuePrefetch(ids []int64, front bool) {
	add := map[int64]bool{}
	var list []int64
	for _, id := range ids {
		if !add[id] && !m.isFresh(id) {
			add[id] = true
			list = append(list, id)
		}
	}
	rest := m.prefetchQueue[:0:0]
	for _, id := range m.prefetchQueue {
		if !add[id] {
			rest = append(rest, id)
		}
	}
	if front {
		m.prefetchQueue = append(list, rest...)
	} else {
		m.prefetchQueue = append(rest, list...)
	}
	m.pumpPrefetch()
}

func (m *model) pumpPrefetch() {
	for m.prefetchRunning < prefetchLimit && m.cli.path != "" && len(m.prefetchQueue) > 0 {
		id := m.prefetchQueue[0]
		m.prefetchQueue = m.prefetchQueue[1:]
		if m.isFresh(id) || m.inFlight[id] {
			continue
		}
		m.prefetchRunning++
		m.fetchThread(id, true)
	}
}

func (m *model) prefetchDone() {
	m.prefetchRunning--
	m.pumpPrefetch()
}

// selectionChanged shows the selected thread once the selection settles, so pressing j
// repeatedly doesn't lay out or load every thread along the way.
func (m *model) selectionChanged() {
	id := m.selected
	time.AfterFunc(70*time.Millisecond, func() {
		m.post(func() {
			if m.selected != id {
				return
			}
			m.shown = id
			m.prefetchNeighbors(id)
			if m.isFresh(id) {
				return
			}
			if _, ok := m.threads[id]; ok {
				m.fetchThread(id, false)
				return
			}
			// Show the disk copy, even if out of date, while fetching the latest.
			go func() {
				t, ok := loadCachedThread(id)
				var entries []entry
				if ok {
					var err error
					entries, err = parseThread(t.JSON, t.HTML)
					ok = err == nil
				}
				m.post(func() {
					if _, loaded := m.threads[id]; ok && !loaded {
						m.threads[id] = threadSnapshot{t.Version, entries}
					}
					m.fetchThread(id, false)
				})
			}()
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
		opener := "open"
		if runtime.GOOS == "linux" {
			opener = "xdg-open"
		}
		_ = exec.Command(opener, url).Start()
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
