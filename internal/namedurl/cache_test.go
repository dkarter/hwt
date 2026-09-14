package namedurl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dkarter/hwt/internal/config"
)

func TestReadCachedURLRemovesExpiredEntry(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	options := Options{Name: "preview", Branch: "main"}
	urlEntry := config.NamedURL{Template: "https://example.com", Cache: config.URLCache{TTL: config.Duration(time.Minute)}}
	cfg := config.Config{URLs: map[string]config.NamedURL{"preview": urlEntry}}
	path, err := urlCachePath("/repo", options, cfg, urlEntry, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(cacheEntry{Result: Result{Name: "preview", URL: "https://example.com"}, Expires: time.Now().Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, _, found := readCachedURL("/repo", options, cfg, urlEntry, false); found {
		t.Fatal("expired cache entry was returned")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expired cache file remains: %v", err)
	}
}
