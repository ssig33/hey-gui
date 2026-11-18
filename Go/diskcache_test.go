package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// useTempCacheDir points os.UserCacheDir at a temporary directory.
func useTempCacheDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, ".cache"))
	if !strings.HasPrefix(cacheDir(), tmp) {
		t.Skipf("cache dir %s is not under the test's temporary directory", cacheDir())
	}
}

func TestPruneCachedThreads(t *testing.T) {
	useTempCacheDir(t)
	for _, id := range []int64{1, 2, 3} {
		saveCachedThread(id, cachedThread{Version: "v", JSON: []byte(`{}`)})
	}
	other := filepath.Join(threadsCacheDir(), "notes.txt")
	if err := os.WriteFile(other, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	pruneCachedThreads(map[int64]bool{2: true})

	files, err := os.ReadDir(threadsCacheDir())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range files {
		names = append(names, f.Name())
	}
	sort.Strings(names)
	if got, want := strings.Join(names, ","), "2.json,notes.txt"; got != want {
		t.Errorf("files after prune = %s, want %s", got, want)
	}
	if th, ok := loadCachedThread(2); !ok || th.Version != "v" {
		t.Errorf("kept thread should still load, got %+v, %v", th, ok)
	}
}

func TestPruneCachedThreadsWithoutCacheDir(t *testing.T) {
	useTempCacheDir(t)
	pruneCachedThreads(map[int64]bool{}) // must not fail when nothing was cached yet
}
