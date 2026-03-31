package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

const repoAPI = "https://api.github.com/repos/cotta-dev/retri/releases/latest"

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

	rel, err := fetchLatest()
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

	dest := "/tmp/" + name
	fmt.Printf("Downloading %s...\n", name)
	if err := download(url, dest); err != nil {
		return fmt.Errorf("download failed: %w", err)
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

func fetchLatest() (*release, error) {
	resp, err := http.Get(repoAPI) //nolint:noctx
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

func download(url, dest string) error {
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	_, err = io.Copy(f, resp.Body)
	return err
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
