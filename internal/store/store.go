// Package store keeps mindful-yt's settings (config.json) and download history
// (history.jsonl), and knows which folders mindful-yt uses.
package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// Config is config.json.
type Config struct {
	OutputDir       string `json:"output_dir"`
	DailyLimit      int    `json:"daily_limit"`
	MaxHeight       int    `json:"max_height"`
	MinReasonLength int    `json:"min_reason_length"`
	SearchResults   int    `json:"search_results"` // how many videos a search lists
}

// DefaultConfig is written to config.json on first run.
var DefaultConfig = Config{
	OutputDir:       defaultOutputDir(runtime.GOOS),
	DailyLimit:      3,
	MaxHeight:       1080,
	MinReasonLength: 10,
	SearchResults:   15,
}

// maxSearchResults caps search_results: each 20 or so more is another request
// to YouTube, and a long list is more to scroll through than to choose from.
const maxSearchResults = 50

// defaultOutputDir is a YT-Saved folder where the OS keeps videos.
func defaultOutputDir(goos string) string {
	switch goos {
	case "windows":
		return `%USERPROFILE%\Videos\YT-Saved`
	case "darwin":
		return "~/Movies/YT-Saved"
	}
	return "~/Videos/YT-Saved"
}

// Entry is one line of history.jsonl.
type Entry struct {
	TS       string  `json:"ts"`
	Date     string  `json:"date"`
	VideoID  string  `json:"video_id"`
	Title    string  `json:"title"`
	Channel  string  `json:"channel"`
	Duration float64 `json:"duration"`
	Quality  int     `json:"quality,omitempty"` // resolution asked for
	Height   int     `json:"height,omitempty"`  // resolution received
	Reason   string  `json:"reason"`
	File     string  `json:"file"`
}

// Store keeps config.json and history.jsonl in one folder.
type Store struct{ Dir string }

// Dirs returns where settings and history live, and where yt-dlp is kept.
// MINDFUL_YT_HOME puts both in one folder, which keeps tests apart.
func Dirs() (data, tools string, err error) {
	if home := os.Getenv("MINDFUL_YT_HOME"); home != "" {
		return home, home, nil
	}
	// %APPDATA%, ~/Library/Application Support, or ~/.config
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", "", err
	}
	// %LOCALAPPDATA%, ~/Library/Caches, or ~/.cache
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", "", err
	}
	return adopt(configDir), adopt(cacheDir), nil
}

// The app was called ytget before it was renamed.
const appDir, oldAppDir = "mindful-yt", "ytget"

// adopt returns base/mindful-yt. If only the old base/ytget folder exists, it
// is renamed first, so settings, history and tools carry over. If the rename
// fails, e.g. because a running ytget has a file open, the old folder keeps
// being used.
func adopt(base string) string {
	dir, old := filepath.Join(base, appDir), filepath.Join(base, oldAppDir)
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		return dir
	}
	if _, err := os.Stat(old); err != nil {
		return dir
	}
	if err := os.Rename(old, dir); err != nil {
		return old
	}
	return dir
}

func (s Store) ConfigPath() string  { return filepath.Join(s.Dir, "config.json") }
func (s Store) HistoryPath() string { return filepath.Join(s.Dir, "history.jsonl") }

// Notepad and PowerShell 5.1 like to prepend a byte order mark when saving.
var bom = []byte("\xef\xbb\xbf")

// LoadConfig reads config.json, writing the defaults first if it's missing.
// Settings left out of the file keep their defaults.
func (s Store) LoadConfig() (Config, error) {
	path := s.ConfigPath()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		data, _ = json.MarshalIndent(DefaultConfig, "", "  ")
		data = append(data, '\n')
		if err := os.MkdirAll(s.Dir, 0o755); err != nil {
			return Config{}, err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return Config{}, err
		}
	} else if err != nil {
		return Config{}, err
	}

	cfg := DefaultConfig
	if err := json.Unmarshal(bytes.TrimPrefix(data, bom), &cfg); err != nil {
		return Config{}, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	cfg.OutputDir = expandPath(cfg.OutputDir)
	if cfg.SearchResults < 1 {
		cfg.SearchResults = DefaultConfig.SearchResults
	}
	cfg.SearchResults = min(cfg.SearchResults, maxSearchResults)
	return cfg, nil
}

var windowsVarRe = regexp.MustCompile(`%([^%]+)%`)

// expandPath expands a leading ~ to the home folder, and %NAME% the way
// Windows does, leaving unknown names alone.
func expandPath(s string) string {
	s = windowsVarRe.ReplaceAllStringFunc(s, func(m string) string {
		if v, ok := os.LookupEnv(m[1 : len(m)-1]); ok {
			return v
		}
		return m
	})
	if s == "~" || strings.HasPrefix(s, "~/") || strings.HasPrefix(s, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			s = home + s[1:]
		}
	}
	return s
}

// ReadHistory returns every readable line of history.jsonl, oldest first.
func (s Store) ReadHistory() ([]Entry, error) {
	data, err := os.ReadFile(s.HistoryPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []Entry
	for _, line := range strings.Split(string(bytes.TrimPrefix(data, bom)), "\n") {
		var e Entry
		if json.Unmarshal([]byte(line), &e) == nil {
			entries = append(entries, e)
		}
	}
	return entries, nil
}

func (s Store) AppendHistory(e Entry) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(s.HistoryPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(e); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Today is the local date in the form history entries use.
func Today() string { return time.Now().Format(time.DateOnly) }

func DownloadsOn(entries []Entry, day string) int {
	n := 0
	for _, e := range entries {
		if e.Date == day {
			n++
		}
	}
	return n
}

// SavedCopies returns the downloads of id whose files still exist, newest
// first, one per resolution.
func SavedCopies(entries []Entry, id string) []Entry {
	var copies []Entry
	seen := map[int]bool{}
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.VideoID != id || e.File == "" || seen[e.Quality] {
			continue
		}
		if _, err := os.Stat(e.File); err != nil {
			continue
		}
		seen[e.Quality] = true
		copies = append(copies, e)
	}
	return copies
}
