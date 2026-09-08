package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"
)

const (
	version      = "0.1.0"
	repoOwner    = "PatrykPanasiuk"
	repoName     = "patcode"
	updateURL    = "https://api.github.com/repos/PatrykPanasiuk/patcode/releases/latest"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for updates and update patcode",
	Long: `Check the latest release on GitHub and update the local binary.
If a new version is found, it downloads and replaces the current binary.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")

		fmt.Printf("patcode %s\n", version)
		fmt.Println("Checking for updates...")

		latest, err := fetchLatestRelease()
		if err != nil {
			return fmt.Errorf("cannot check for updates: %w", err)
		}

		fmt.Printf("  latest release: %s\n", latest.Tag)

		if latest.Tag == "v"+version && !force {
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
	return latestTag != "" && latestTag != "v"+version
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
	assetName := fmt.Sprintf("patcode_%s_%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		assetName += ".exe"
	}

	var downloadURL string
	for _, a := range latest.Assets {
		if a.Name == assetName {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		// Fallback: build from source
		return buildFromSource(latest.Tag)
	}

	return downloadBinary(downloadURL)
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
