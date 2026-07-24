package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const dailyCheckCacheName = "update-check.json"

type dailyCheckCache struct {
	CheckedDate string `json:"checked_date"`
}

type checkOptions struct {
	CacheDir string
	Client   *http.Client
	Now      func() time.Time
	URL      string
	Writer   io.Writer
}

// MaybeNotify checks GitHub Releases at most once per local day and prints a
// terse notice when a newer version exists. Network and cache failures are
// intentionally non-fatal so normal retri execution is never blocked.
func MaybeNotify(currentVersion string, enabled bool) {
	_ = maybeNotify(currentVersion, enabled, checkOptions{})
}

func maybeNotify(currentVersion string, enabled bool, opts checkOptions) error {
	if !enabled || currentVersion == "" || currentVersion == "dev" {
		return nil
	}

	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	today := now().Format("2006-01-02")

	cacheDir := opts.CacheDir
	if cacheDir == "" {
		var err error
		cacheDir, err = defaultCacheDir()
		if err != nil {
			return nil
		}
	}
	cachePath := filepath.Join(cacheDir, dailyCheckCacheName)
	if alreadyCheckedToday(cachePath, today) {
		return nil
	}
	writeCheckedDate(cachePath, today)

	client := opts.Client
	if client == nil {
		client = httpClient
	}
	url := opts.URL
	if url == "" {
		url = repoAPI
	}
	writer := opts.Writer
	if writer == nil {
		writer = os.Stderr
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	rel, err := fetchLatest(ctx, client, url)
	if err != nil {
		return nil
	}

	latest := strings.TrimPrefix(rel.TagName, "v")
	current := strings.TrimPrefix(currentVersion, "v")
	if isNewer(latest, current) {
		_, _ = fmt.Fprintf(writer, "[INFO] New retri version available: v%s\n", latest)
	}
	return nil
}

func defaultCacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "retri"), nil
}

func alreadyCheckedToday(path, today string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var cache dailyCheckCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return false
	}
	return cache.CheckedDate == today
}

func writeCheckedDate(path, today string) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	data, err := json.Marshal(dailyCheckCache{CheckedDate: today})
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0644)
}
