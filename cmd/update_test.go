package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"patcode/version"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a    string
		b    string
		want int
	}{
		{"1.2.3", "1.2.3", 0},
		{"v1.2.3", "1.2.3", 0},
		{"1.2.3", "1.2.4", -1},
		{"1.3.0", "1.2.4", 1},
		{"2.0.0", "1.9.9", 1},
		{"0.10.0", "0.2.0", 1},
		{"1.2", "1.2.0", 0},
		{"1.2.3", "1.2", 1},
		{"v0.1.0", "0.0.9", 1},
		{"1.2.3-rc1", "1.2.3", 0},
	}
	for _, c := range cases {
		got := compareVersions(c.a, c.b)
		if (c.want < 0 && got >= 0) || (c.want > 0 && got <= 0) || (c.want == 0 && got != 0) {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestNeedsUpdate(t *testing.T) {
	old := version.Version
	defer func() { version.Version = old }()
	version.Version = "v0.1.0"

	if NeedsUpdate("0.1.0") {
		t.Error("same version should not need update")
	}
	if NeedsUpdate("v0.1.0") {
		t.Error("v-prefixed same version should not need update")
	}
	if NeedsUpdate("") {
		t.Error("empty tag should not need update")
	}
	if !NeedsUpdate("0.2.0") {
		t.Error("newer version should need update")
	}
	if !NeedsUpdate("v0.10.0") {
		t.Error("0.10.0 should be newer than 0.1.0")
	}
	if NeedsUpdate("0.0.9") {
		t.Error("older version should not need update")
	}
}

func TestPickReleaseAsset(t *testing.T) {
	rel := &githubRelease{Tag: "v0.2.0", Assets: []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	}{
		{Name: "checksums.txt", BrowserDownloadURL: "https://x/checksums.txt"},
		{Name: "patcode_v0.2.0_linux_amd64.tar.gz", BrowserDownloadURL: "https://x/linux.tgz"},
		{Name: "patcode_v0.2.0_darwin_arm64.tar.gz", BrowserDownloadURL: "https://x/darwin.tgz"},
	}}

	url, isArchive := pickReleaseAsset(rel, "linux", "amd64", "v0.2.0")
	if url != "https://x/linux.tgz" || !isArchive {
		t.Errorf("linux: got (%q, %v)", url, isArchive)
	}

	url, isArchive = pickReleaseAsset(rel, "windows", "amd64", "v0.2.0")
	if url != "" {
		t.Errorf("windows: expected no asset, got %q", url)
	}
}

func TestPickReleaseAsset_RawBinaryFallback(t *testing.T) {
	rel := &githubRelease{Tag: "v0.2.0", Assets: []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	}{
		{Name: "patcode_windows_amd64.exe", BrowserDownloadURL: "https://x/win.exe"},
	}}

	url, isArchive := pickReleaseAsset(rel, "windows", "amd64", "v0.2.0")
	if url != "https://x/win.exe" || isArchive {
		t.Errorf("windows raw: got (%q, %v)", url, isArchive)
	}
}

func TestUpdateCacheRoundtrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	now := time.Now()
	cache := updateCheckCache{CheckedAt: now, LatestVersion: "1.2.3"}
	saveUpdateCache(cache)

	loaded, ok := loadUpdateCache()
	if !ok {
		t.Fatal("expected cache to load")
	}
	if !loaded.CheckedAt.Equal(now) {
		t.Errorf("checked_at mismatch: %v != %v", loaded.CheckedAt, now)
	}
	if loaded.LatestVersion != "1.2.3" {
		t.Errorf("latest_version mismatch: %q", loaded.LatestVersion)
	}

	path := filepath.Join(home, ".patcode", "update_check.json")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected cache file at %s: %v", path, err)
	}
}
