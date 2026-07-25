package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const repoAPI = "https://api.github.com/repos/cotta-dev/retri/releases/latest"

const (
	maxUpdateSize         = 256 << 20
	aptSandboxPackageMode = 0o644
)

var (
	httpClient     = &http.Client{Timeout: 3 * time.Second}
	downloadClient = &http.Client{Timeout: 2 * time.Minute}
)

type release struct {
	TagName string  `json:"tag_name"`
	Assets  []asset `json:"assets"`
}

type asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Run checks for a newer release and installs it if available.
func Run(currentVersion string) error {
	fmt.Println("Checking for updates...")

	fetchCtx, fetchCancel := context.WithTimeout(context.Background(), 5*time.Second)
	rel, err := fetchLatest(fetchCtx, httpClient, repoAPI)
	fetchCancel()
	if err != nil {
		return fmt.Errorf("failed to fetch release info: %w", err)
	}

	latest := strings.TrimPrefix(rel.TagName, "v")
	current := strings.TrimPrefix(currentVersion, "v")

	if !isNewer(latest, current) {
		fmt.Printf("Already up to date (v%s)\n", current)
		return nil
	}

	fmt.Printf("New version available: v%s (current: v%s)\n", latest, current)

	arch := goArchToDebian(runtime.GOARCH)
	url, name := findDebAsset(rel.Assets, arch)
	if url == "" {
		return fmt.Errorf("no .deb package found for architecture %s", arch)
	}
	if err := validateReleaseDownloadURL(url); err != nil {
		return err
	}

	fmt.Printf("Downloading %s...\n", name)
	f, err := os.CreateTemp("", "retri-update-*.deb")
	if err != nil {
		return fmt.Errorf("create secure temporary package: %w", err)
	}
	dest := f.Name()
	fileOpen := true
	defer func() {
		if fileOpen {
			_ = f.Close()
		}
		_ = os.Remove(dest)
	}()

	downloadCtx, downloadCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer downloadCancel()
	if err := download(downloadCtx, downloadClient, url, f); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync downloaded package: %w", err)
	}
	if err := makePackageReadableByAPT(f); err != nil {
		return fmt.Errorf("prepare package for apt sandbox: %w", err)
	}
	closeErr := f.Close()
	fileOpen = false
	if closeErr != nil {
		return fmt.Errorf("close downloaded package: %w", closeErr)
	}

	fmt.Println("Installing...")
	cmd := exec.Command("sudo", "apt-get", "install", "-y", dest)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("installation failed: %w", err)
	}

	fmt.Printf("Successfully updated to v%s\n", latest)
	return nil
}

// makePackageReadableByAPT grants the unprivileged _apt sandbox read access
// only after the public release package has been downloaded and synced.
func makePackageReadableByAPT(file *os.File) error {
	return file.Chmod(aptSandboxPackageMode)
}

func fetchLatest(ctx context.Context, client *http.Client, url string) (*release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

func download(ctx context.Context, client *http.Client, url string, dst io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}
	if resp.Request == nil || resp.Request.URL == nil || resp.Request.URL.Scheme != "https" {
		return fmt.Errorf("download redirected to an insecure URL")
	}
	if resp.ContentLength > maxUpdateSize {
		return fmt.Errorf("package is too large: %d bytes", resp.ContentLength)
	}

	limited := io.LimitReader(resp.Body, maxUpdateSize+1)
	n, err := io.Copy(dst, limited)
	if err != nil {
		return err
	}
	if n > maxUpdateSize {
		return fmt.Errorf("package exceeds %d bytes", maxUpdateSize)
	}
	return nil
}

func validateReleaseDownloadURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid release download URL: %w", err)
	}
	if parsed.Scheme != "https" || !strings.EqualFold(parsed.Hostname(), "github.com") ||
		!strings.HasPrefix(parsed.EscapedPath(), "/cotta-dev/retri/releases/download/") {
		return fmt.Errorf("untrusted release download URL %q", rawURL)
	}
	return nil
}

func findDebAsset(assets []asset, arch string) (url, name string) {
	for _, a := range assets {
		if strings.HasSuffix(a.Name, arch+".deb") {
			return a.BrowserDownloadURL, a.Name
		}
	}
	return "", ""
}

func goArchToDebian(goarch string) string {
	if goarch == "arm64" {
		return "arm64"
	}
	return "amd64"
}

// isNewer returns true if latest > current using semver comparison.
func isNewer(latest, current string) bool {
	lp := parseSemver(latest)
	cp := parseSemver(current)
	for i := range lp {
		if lp[i] > cp[i] {
			return true
		}
		if lp[i] < cp[i] {
			return false
		}
	}
	return false
}

func parseSemver(v string) [3]int {
	parts := strings.SplitN(v, ".", 3)
	var result [3]int
	for i, p := range parts {
		if i >= 3 {
			break
		}
		result[i], _ = strconv.Atoi(p)
	}
	return result
}
