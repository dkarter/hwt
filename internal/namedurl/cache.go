package namedurl

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/dkarter/hwt/internal/config"
	"github.com/dkarter/hwt/internal/pullrequest"
)

type cacheEntry struct {
	Result   Result    `json:"result"`
	NotFound bool      `json:"not_found,omitempty"`
	Expires  time.Time `json:"expires"`
}

func readCachedURL(root string, options Options, cfg config.Config, entry config.NamedURL, branchDependent bool) (Result, error, bool) {
	path, err := urlCachePath(root, options, cfg, entry, branchDependent)
	if err != nil {
		return Result{}, nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Result{}, nil, false
	}
	var cached cacheEntry
	if json.Unmarshal(data, &cached) != nil || !time.Now().Before(cached.Expires) {
		_ = os.Remove(path)
		return Result{}, nil, false
	}
	if cached.NotFound {
		return Result{}, pullrequest.ErrNotFound, true
	}
	if cached.Result.Name != options.Name || cached.Result.URL == "" {
		return Result{}, nil, false
	}
	return cached.Result, nil, true
}

func writeCachedURL(root string, options Options, cfg config.Config, urlEntry config.NamedURL, branchDependent bool, result Result, notFound bool, ttl time.Duration) {
	path, err := urlCachePath(root, options, cfg, urlEntry, branchDependent)
	if err != nil || ttl <= 0 {
		return
	}
	now := time.Now()
	data, err := json.Marshal(cacheEntry{Result: result, NotFound: notFound, Expires: now.Add(ttl)})
	dir := filepath.Dir(path)
	if err != nil || os.MkdirAll(dir, 0o700) != nil {
		return
	}
	pruneURLCache(dir, now)
	temporary, err := os.CreateTemp(dir, ".url-*")
	if err != nil {
		return
	}
	temporaryPath := temporary.Name()
	defer temporary.Close()
	defer os.Remove(temporaryPath)
	if temporary.Chmod(0o600) != nil || writeCacheFile(temporary, data) != nil {
		return
	}
	_ = os.Rename(temporaryPath, path)
}

func removeCachedURL(root string, options Options, cfg config.Config, entry config.NamedURL, branchDependent bool) {
	path, err := urlCachePath(root, options, cfg, entry, branchDependent)
	if err == nil {
		_ = os.Remove(path)
	}
}

func urlCachePath(root string, options Options, cfg config.Config, entry config.NamedURL, branchDependent bool) (string, error) {
	cacheRoot := os.Getenv("XDG_CACHE_HOME")
	if cacheRoot == "" {
		var err error
		cacheRoot, err = os.UserCacheDir()
		if err != nil {
			return "", err
		}
	}
	branch := ""
	if branchDependent {
		branch = options.Branch
	}
	key, err := json.Marshal(struct {
		Root       string
		Name       string
		Branch     string
		Repository string
		Entry      config.NamedURL
		Metadata   config.Metadata
		LocalDNS   config.LocalDNS
	}{root, options.Name, branch, options.Repository, entry, cfg.Metadata, cfg.LocalDNS})
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(key)
	return filepath.Join(cacheRoot, "hwt", "urls", hex.EncodeToString(hash[:])+".json"), nil
}

func pruneURLCache(dir string, now time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, item := range entries {
		if item.IsDir() {
			continue
		}
		path := filepath.Join(dir, item.Name())
		data, err := os.ReadFile(path)
		var cached cacheEntry
		if err != nil || json.Unmarshal(data, &cached) != nil || !now.Before(cached.Expires) {
			_ = os.Remove(path)
		}
	}
}

func writeCacheFile(file *os.File, data []byte) error {
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
