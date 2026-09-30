package store

import (
	"os"
	"path/filepath"
	"testing"
)

const testID = "jNQXAC9IVRw"

func TestConfigDefaultsAreWrittenAndBOMIsAccepted(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	cfg, err := s.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DailyLimit != 3 || cfg.MaxHeight != 1080 || cfg.MinReasonLength != 10 {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
	if _, err := os.Stat(s.ConfigPath()); err != nil {
		t.Fatal("config.json wasn't written")
	}

	// Saved from Notepad: BOM first, only one setting changed.
	if err := os.WriteFile(s.ConfigPath(), []byte("\xef\xbb\xbf{\"daily_limit\": 7}"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = s.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DailyLimit != 7 || cfg.MaxHeight != 1080 {
		t.Errorf("got %+v", cfg)
	}
}

func TestBrokenConfigIsReported(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	os.WriteFile(s.ConfigPath(), []byte("{nope"), 0o644)
	if _, err := s.LoadConfig(); err == nil {
		t.Fatal("broken JSON was accepted")
	}
}

func TestOutputPathsExpand(t *testing.T) {
	t.Setenv("MINDFUL_YT_TEST_DIR", `C:\Users\someone`)
	if got := expandPath(`%MINDFUL_YT_TEST_DIR%\Videos`); got != `C:\Users\someone\Videos` {
		t.Errorf("got %q", got)
	}
	if got := expandPath(`%MINDFUL_YT_NOT_SET%\x`); got != `%MINDFUL_YT_NOT_SET%\x` {
		t.Errorf("unknown variable changed: %q", got)
	}
	home, _ := os.UserHomeDir()
	if got := expandPath("~/Videos/YT-Saved"); got != home+"/Videos/YT-Saved" {
		t.Errorf("got %q", got)
	}
	if got := expandPath("/srv/~/x"); got != "/srv/~/x" {
		t.Errorf("a ~ that isn't leading changed: %q", got)
	}
}

func TestDefaultOutputDirPerOS(t *testing.T) {
	for goos, want := range map[string]string{
		"windows": `%USERPROFILE%\Videos\YT-Saved`,
		"darwin":  "~/Movies/YT-Saved",
		"linux":   "~/Videos/YT-Saved",
	} {
		if got := defaultOutputDir(goos); got != want {
			t.Errorf("%s: got %q, want %q", goos, got, want)
		}
	}
}

func TestHistoryReadsPythonEraLinesAndCountsToday(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	lines := `{"ts": "2026-09-29T10:00:00", "date": "2026-09-29", "video_id": "aaaaaaaaaaa", "title": "Old", "channel": "x", "duration": 19, "reason": "r", "file": null}
not json
{"ts": "t", "date": "` + Today() + `", "video_id": "bbbbbbbbbbb", "quality": 720, "height": 720, "reason": "r", "file": "f"}
{"date": "` + Today() + `"}
`
	os.WriteFile(s.HistoryPath(), []byte(lines), 0o644)
	entries, err := s.ReadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries", len(entries))
	}
	if got := DownloadsOn(entries, Today()); got != 2 {
		t.Errorf("today: got %d, want 2", got)
	}
}

func TestAppendHistoryRoundTrips(t *testing.T) {
	s := Store{Dir: filepath.Join(t.TempDir(), "new")}
	e := Entry{Date: Today(), VideoID: testID, Title: "Tom & Jerry <강남스타일>", Quality: 1080, Reason: "why not"}
	if err := s.AppendHistory(e); err != nil {
		t.Fatal(err)
	}
	entries, _ := s.ReadHistory()
	if len(entries) != 1 || entries[0] != e {
		t.Fatalf("got %+v", entries)
	}
}

func TestSavedCopiesAreOnePerResolutionNewestFirst(t *testing.T) {
	dir := t.TempDir()
	file := func(name string) string {
		path := filepath.Join(dir, name)
		os.WriteFile(path, nil, 0o644)
		return path
	}
	older, newer, other := file("a.mp4"), file("b.mp4"), file("c.mp4")
	entries := []Entry{
		{VideoID: testID, Quality: 1080, File: older},
		{VideoID: testID, Quality: 720, File: filepath.Join(dir, "deleted.mp4")},
		{VideoID: testID, Quality: 1080, File: newer},
		{VideoID: "someoneelse", Quality: 1080, File: other},
	}
	got := SavedCopies(entries, testID)
	if len(got) != 1 || got[0].File != newer {
		t.Fatalf("got %+v", got)
	}
}

func TestOldYtgetFolderMovesToTheNewName(t *testing.T) {
	base := t.TempDir()
	old := filepath.Join(base, "ytget")
	os.MkdirAll(old, 0o755)
	os.WriteFile(filepath.Join(old, "history.jsonl"), []byte("today's downloads\n"), 0o644)

	dir := adopt(base)
	if dir != filepath.Join(base, "mindful-yt") {
		t.Fatalf("got %q", dir)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "history.jsonl")); string(got) != "today's downloads\n" {
		t.Errorf("history didn't carry over: %q", got)
	}
	if _, err := os.Stat(old); err == nil {
		t.Error("the old folder is still there")
	}
}

func TestExistingFolderIsNeverReplaced(t *testing.T) {
	base := t.TempDir()
	for _, name := range []string{"ytget", "mindful-yt"} {
		os.MkdirAll(filepath.Join(base, name), 0o755)
		os.WriteFile(filepath.Join(base, name, "config.json"), []byte(name), 0o644)
	}
	if dir := adopt(base); dir != filepath.Join(base, "mindful-yt") {
		t.Fatalf("got %q", dir)
	}
	if got, _ := os.ReadFile(filepath.Join(base, "mindful-yt", "config.json")); string(got) != "mindful-yt" {
		t.Errorf("the new folder was overwritten: %q", got)
	}
	if _, err := os.Stat(filepath.Join(base, "ytget")); err != nil {
		t.Error("the old folder was touched even though the new one exists")
	}
}

func TestFreshInstallCreatesNothing(t *testing.T) {
	base := t.TempDir()
	if dir := adopt(base); dir != filepath.Join(base, "mindful-yt") {
		t.Fatalf("got %q", dir)
	}
	if entries, _ := os.ReadDir(base); len(entries) != 0 {
		t.Errorf("adopt created %v", entries)
	}
}

func TestMindfulYtHomeKeepsEverythingTogether(t *testing.T) {
	t.Setenv("MINDFUL_YT_HOME", `C:\somewhere`)
	data, tools, err := Dirs()
	if err != nil || data != `C:\somewhere` || tools != `C:\somewhere` {
		t.Errorf("got %q, %q, %v", data, tools, err)
	}
}
