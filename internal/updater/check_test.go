package updater

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestMaybeNotifyPrintsNewVersionAndCachesByDay(t *testing.T) {
	t.Parallel()

	var requests int
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests++
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(bytes.NewBufferString(`{"tag_name":"v1.2.0","assets":[]}`)),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}),
	}

	var out bytes.Buffer
	cacheDir := t.TempDir()
	now := func() time.Time { return time.Date(2026, 5, 22, 9, 0, 0, 0, time.Local) }
	opts := checkOptions{
		CacheDir: cacheDir,
		Client:   client,
		Now:      now,
		URL:      "https://example.invalid/latest",
		Writer:   &out,
	}

	if err := maybeNotify("1.1.0", true, opts); err != nil {
		t.Fatalf("maybeNotify returned error: %v", err)
	}
	if got := out.String(); got != "[INFO] New retri version available: v1.2.0\n" {
		t.Fatalf("unexpected output: %q", got)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}

	out.Reset()
	if err := maybeNotify("1.1.0", true, opts); err != nil {
		t.Fatalf("maybeNotify returned error: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("second check output = %q, want empty", out.String())
	}
	if requests != 1 {
		t.Fatalf("requests after cached check = %d, want 1", requests)
	}

	if !alreadyCheckedToday(filepath.Join(cacheDir, dailyCheckCacheName), "2026-05-22") {
		t.Fatal("cache was not written for today")
	}
}

func TestMaybeNotifyCanBeDisabled(t *testing.T) {
	t.Parallel()

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			t.Fatal("client should not be called when update check is disabled")
			return nil, nil
		}),
	}

	var out bytes.Buffer
	err := maybeNotify("1.1.0", false, checkOptions{
		CacheDir: t.TempDir(),
		Client:   client,
		Now:      func() time.Time { return time.Date(2026, 5, 22, 9, 0, 0, 0, time.Local) },
		URL:      "https://example.invalid/latest",
		Writer:   &out,
	})
	if err != nil {
		t.Fatalf("maybeNotify returned error: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("output = %q, want empty", out.String())
	}
}

func TestMaybeNotifyIgnoresNetworkErrors(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("network unavailable")
		}),
	}
	err := maybeNotify("1.1.0", true, checkOptions{
		CacheDir: t.TempDir(),
		Client:   client,
		Now:      func() time.Time { return time.Date(2026, 5, 22, 9, 0, 0, 0, time.Local) },
		URL:      "https://example.invalid/latest",
		Writer:   &out,
	})
	if err != nil {
		t.Fatalf("maybeNotify returned error: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("output = %q, want empty", out.String())
	}
}
