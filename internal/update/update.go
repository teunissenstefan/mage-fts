// Package update reports whether a newer mage-fts has been released.
//
// It only ever tells the user; it never replaces the binary. mage-fts is installed
// through Homebrew for most people, and a program that overwrites its own
// Homebrew-managed binary leaves brew reporting a version that is not on disk.
// Printing "run brew upgrade" is both honest and enough.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// releasesURL is the unauthenticated GitHub API. Rate limited to 60
	// requests an hour per IP, which the cache keeps us far below.
	releasesURL = "https://api.github.com/repos/epenthesis/mage-fts/releases/latest"

	// checkInterval is how long a result is trusted. A day is frequent enough
	// for a tool nobody runs continuously.
	checkInterval = 24 * time.Hour

	// timeout bounds the request. The check runs in the background, but a
	// hung connection should not keep a goroutine alive for the session.
	timeout = 3 * time.Second

	// DisableEnv turns the check off entirely.
	DisableEnv = "MAGE_FTS_NO_UPDATE_CHECK"
)

// Result reports the outcome. Every failure path returns a zero Result and a
// nil error: not knowing about an update is not a problem worth showing.
type Result struct {
	Available bool
	Latest    string // version without the leading "v"
}

type cache struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
}

// Check reports whether a release newer than current exists, consulting a
// cached answer before the network.
//
// It returns a zero Result for development builds: an unreleased binary has no
// meaningful version to compare, and telling someone running their own build
// to upgrade is noise.
func Check(ctx context.Context, current string) Result {
	if os.Getenv(DisableEnv) != "" {
		return Result{}
	}
	if !comparable(current) {
		return Result{}
	}

	latest, ok := cached()
	if !ok {
		var err error
		latest, err = fetch(ctx)
		if err != nil {
			return Result{}
		}
		store(latest)
	}

	if latest == "" || !newer(latest, current) {
		return Result{}
	}
	return Result{Available: true, Latest: latest}
}

// comparable reports whether a version string is a real release. Local builds
// report "dev", and goreleaser snapshots carry a SNAPSHOT suffix.
func comparable(v string) bool {
	if v == "" || v == "dev" {
		return false
	}
	return !strings.Contains(v, "SNAPSHOT")
}

func cachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "mage-fts", "update.json"), nil
}

// cached returns the stored version if it is still fresh.
func cached() (string, bool) {
	path, err := cachePath()
	if err != nil {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var c cache
	if json.Unmarshal(data, &c) != nil {
		return "", false
	}
	if time.Since(c.CheckedAt) > checkInterval {
		return "", false
	}
	return c.Latest, true
}

// store writes the result. Failures are ignored: an unwritable cache means one
// request per run, not a broken program.
func store(latest string) {
	path, err := cachePath()
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	data, err := json.Marshal(cache{CheckedAt: time.Now(), Latest: latest})
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}

func fetch(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github returned %s", resp.Status)
	}

	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return strings.TrimPrefix(body.TagName, "v"), nil
}

// newer reports whether latest is a higher version than current, comparing
// numerically so that 0.10.0 beats 0.9.0. Anything unparseable falls back to
// inequality, which errs towards telling the user something changed.
func newer(latest, current string) bool {
	l, lok := parse(latest)
	c, cok := parse(current)
	if !lok || !cok {
		return latest != current
	}
	for i := range 3 {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

// parse splits "1.2.3" into its numeric parts, ignoring any pre-release or
// build suffix.
func parse(v string) ([3]int, bool) {
	var out [3]int

	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}

	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
