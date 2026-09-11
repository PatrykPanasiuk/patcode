package cmd

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"patcode/version"
)

const (
	repoOwner = "PatrykPanasiuk"
	repoName  = "patcode"
	updateURL = "https://api.github.com/repos/PatrykPanasiuk/patcode/releases/latest"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for updates and update patcode",
	Long: `Check the latest release on GitHub and update the local binary.
If a new version is found, it downloads and replaces the current binary.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")

		fmt.Printf("patcode %s\n", version.Version)
		fmt.Println("Checking for updates...")

		latest, err := fetchLatestRelease()
		if err != nil {
			return fmt.Errorf("cannot check for updates: %w", err)
		}

		fmt.Printf("  latest release: %s\n", latest.Tag)

		if latest.Tag != "" && !force && compareVersions(latest.Tag, version.Version) <= 0 {
			fmt.Println("  you are already up to date.")
			return nil
		}

		if latest.Tag == "" {
			fmt.Println("  no releases published yet.")
			fmt.Printf("  Build from source: https://github.com/%s/%s\n", repoOwner, repoName)
			return nil
		}

		fmt.Printf("  downloading %s...\n", latest.Tag)
		if err := downloadAndInstall(latest); err != nil {
			return fmt.Errorf("update failed: %w", err)
		}

		fmt.Println("  update complete.")
		return nil
	},
}

// CheckUpdate fetches the latest release tag silently.
// Returns the latest tag, or empty string on any error.
func CheckUpdate() string {
	latest, err := fetchLatestRelease()
	if err != nil {
		return ""
	}
	return latest.Tag
}

func NeedsUpdate(latestTag string) bool {
	return latestTag != "" && compareVersions(latestTag, version.Version) > 0
}

// compareVersions compares two version strings such as "1.2.3", "v1.2.3" or
// "0.10.0". Thenumeric segments are compared in order; missing segments count
// as zero. It returns -1, 0 or 1 like strings.Compare. Prerelease/extra
// suffixes after a trailing "-" or "+" are ignored.
func compareVersions(a, b string) int {
	an := parseVersion(a)
	bn := parseVersion(b)
	n := len(an)
	if len(bn) > n {
		n = len(bn)
	}
	for i := 0; i < n; i++ {
		var av, bv int
		if i < len(an) {
			av = an[i]
		}
		if i < len(bn) {
			bv = bn[i]
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}

func parseVersion(s string) []int {
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	var out []int
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			break
		}
		out = append(out, n)
	}
	return out
}

const updateCheckInterval = 24 * time.Hour

type updateCheckCache struct {
	CheckedAt     time.Time `json:"checked_at"`
	LatestVersion string    `json:"latest_version"`
}

func updateCachePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".patcode", "update_check.json")
}

func loadUpdateCache() (updateCheckCache, bool) {
	data, err := os.ReadFile(updateCachePath())
	if err != nil {
		return updateCheckCache{}, false
	}
	var cache updateCheckCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return updateCheckCache{}, false
	}
	return cache, true
}

func saveUpdateCache(cache updateCheckCache) {
	path := updateCachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}

// MaybeCheckForUpdates runs at startup. When a cached check from the last
// 24h already found a newer release it prints the notice immediately (no
// network). Otherwise it refreshes the check in the background so startup
// stays fast.
func MaybeCheckForUpdates() {
	if version.Version == "" || strings.Contains(version.Version, "-") {
		return
	}
	cache, ok := loadUpdateCache()
	if ok && time.Since(cache.CheckedAt) < updateCheckInterval {
		if NeedsUpdate(cache.LatestVersion) {
			printUpdateNotice(cache.LatestVersion)
		}
		return
	}
	go refreshUpdateCheck()
}

func refreshUpdateCheck() {
	cache := updateCheckCache{
		CheckedAt:     time.Now(),
		LatestVersion: CheckUpdate(),
	}
	saveUpdateCache(cache)
	if NeedsUpdate(cache.LatestVersion) {
		printUpdateNotice(cache.LatestVersion)
	}
}

func printUpdateNotice(latest string) {
	fmt.Fprintf(os.Stderr, "\nA new patcode version is available: %s \u2192 %s\n",
		version.Version, latest)
	fmt.Fprintln(os.Stderr, "Run `patcode update` to upgrade.")
}

type githubRelease struct {
	Tag    string `json:"tag_name"`
	Assets []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func fetchLatestRelease() (*githubRelease, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", updateURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		// No releases yet — return empty
		return &githubRelease{}, nil
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &rel, nil
}

func downloadAndInstall(latest *githubRelease) error {
	url, isArchive := pickReleaseAsset(latest, runtime.GOOS, runtime.GOARCH, latest.Tag)
	if url == "" {
		return buildFromSource(latest.Tag)
	}
	if isArchive {
		return downloadAndExtractArchive(url, latest.Tag)
	}
	return downloadBinary(url)
}

func pickReleaseAsset(latest *githubRelease, osName, arch, tag string) (string, bool) {
	assetName := fmt.Sprintf("patcode_%s_%s", osName, arch)
	if osName == "windows" {
		assetName += ".exe"
	}
	archiveName := fmt.Sprintf("patcode_%s_%s_%s.tar.gz", tag, osName, arch)

	var downloadURL string
	downloadsArchive := false
	for _, a := range latest.Assets {
		switch a.Name {
		case archiveName:
			downloadURL = a.BrowserDownloadURL
			downloadsArchive = true
		case assetName:
			if downloadURL == "" {
				downloadURL = a.BrowserDownloadURL
			}
		}
	}
	return downloadURL, downloadsArchive
}

func downloadAndExtractArchive(url, tag string) error {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("opening archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine executable path: %w", err)
	}

	binName := "patcode"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading archive: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg || filepath.Base(hdr.Name) != binName {
			continue
		}

		tmpPath := exe + ".tmp"
		f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return fmt.Errorf("cannot create temp file: %w", err)
		}
		written, err := io.Copy(f, tr)
		closeErr := f.Close()
		if err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("download write error: %w", err)
		}
		if closeErr != nil {
			os.Remove(tmpPath)
			return closeErr
		}
		if err := os.Rename(tmpPath, exe); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("cannot replace binary: %w", err)
		}
		fmt.Printf("  installed patcode %s (wrote %d bytes)\n", tag, written)
		return nil
	}

	return fmt.Errorf("archive contains no %s binary", binName)
}

func downloadBinary(url string) error {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine executable path: %w", err)
	}

	tmpPath := exe + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}

	written, err := io.Copy(f, resp.Body)
	if err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("download write error: %w", err)
	}
	f.Close()

	if err := os.Rename(tmpPath, exe); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("cannot replace binary: %w", err)
	}

	fmt.Printf("  wrote %d bytes\n", written)
	return nil
}

func buildFromSource(tag string) error {
	tmpDir, err := os.MkdirTemp("", "patcode-update")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", repoOwner, repoName)
	fmt.Printf("  cloning %s %s...\n", cloneURL, tag)

	clone := exec.Command("git", "clone", "--depth", "1", "--branch", tag, cloneURL, tmpDir)
	clone.Stdout = os.Stdout
	clone.Stderr = os.Stderr
	if err := clone.Run(); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}

	fmt.Println("  building...")
	build := exec.Command("go", "build", "-o", "patcode", ".")
	build.Dir = tmpDir
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine executable path: %w", err)
	}

	srcExe := filepath.Join(tmpDir, "patcode")
	if runtime.GOOS == "windows" {
		srcExe += ".exe"
	}

	input, err := os.ReadFile(srcExe)
	if err != nil {
		return err
	}
	if err := os.WriteFile(exe, input, 0755); err != nil {
		return fmt.Errorf("cannot replace binary: %w", err)
	}

	fmt.Printf("  updated %s\n", exe)
	return nil
}

func init() {
	updateCmd.Flags().Bool("force", false, "force reinstall even if version matches")
	rootCmd.AddCommand(updateCmd)
}
