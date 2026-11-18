package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Raw hey CLI output kept in hey-gui under os.UserCacheDir (~/Library/Caches on macOS,
// ~/.cache on Linux) so the list and threads show instantly on the next launch.

func cacheDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "hey-gui")
}

func threadsCacheDir() string { return filepath.Join(cacheDir(), "threads") }

func loadCachedImbox() []byte {
	data, _ := os.ReadFile(filepath.Join(cacheDir(), "imbox.json"))
	return data
}

func saveCachedImbox(data []byte) {
	writeCacheFile(filepath.Join(cacheDir(), "imbox.json"), data)
}

// cachedThread is one thread's `hey thread read --json` and `--html` output, tagged with
// the list version it was fetched for.
type cachedThread struct {
	Version string `json:"version"`
	JSON    []byte `json:"json"`
	HTML    []byte `json:"html,omitempty"`
}

func threadCachePath(topicID int64) string {
	return filepath.Join(threadsCacheDir(), fmt.Sprintf("%d.json", topicID))
}

func loadCachedThread(topicID int64) (cachedThread, bool) {
	var t cachedThread
	data, err := os.ReadFile(threadCachePath(topicID))
	if err != nil || json.Unmarshal(data, &t) != nil {
		return t, false
	}
	return t, true
}

func saveCachedThread(topicID int64, t cachedThread) {
	if data, err := json.Marshal(t); err == nil {
		writeCacheFile(threadCachePath(topicID), data)
	}
}

// pruneCachedThreads deletes cached threads that are no longer in the Imbox.
func pruneCachedThreads(keep map[int64]bool) {
	files, _ := os.ReadDir(threadsCacheDir())
	for _, f := range files {
		id, err := strconv.ParseInt(strings.TrimSuffix(f.Name(), ".json"), 10, 64)
		if err == nil && !keep[id] {
			_ = os.Remove(filepath.Join(threadsCacheDir(), f.Name()))
		}
	}
}

func writeCacheFile(path string, data []byte) {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		_ = os.Rename(tmp, path)
	}
}
