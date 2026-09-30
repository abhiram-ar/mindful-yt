package hosts

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var v6Lines = []string{":: youtube.com", ":: www.youtube.com", ":: m.youtube.com", ":: youtu.be", ":: www.youtu.be"}

func TestCheck(t *testing.T) {
	for _, c := range []struct {
		name    string
		content string
		missing []string
	}{
		{"empty", "", Lines()},
		{"blocked by hand for IPv4, the way it's done on the dev machine",
			"127.0.0.1 youtube.com\r\n127.0.0.1 www.youtube.com\r\n127.0.0.1 m.youtube.com\r\n" +
				"127.0.0.1 youtu.be\r\n127.0.0.1 www.youtu.be\r\n",
			v6Lines},
		{"everything", strings.Join(Lines(), "\n"), nil},
		{"any loopback or unspecified address counts",
			"0.0.0.0 youtube.com www.youtube.com m.youtube.com youtu.be www.youtu.be\n" +
				"::1\tyoutube.com www.youtube.com\n:: m.youtube.com youtu.be\n0:0:0:0:0:0:0:0 www.youtu.be\n",
			nil},
		{"case, trailing dots, tabs, inline comments and a BOM",
			"\xef\xbb\xbf# the usual header\n0.0.0.0\tYouTube.com.   # no more\n\t0.0.0.0 WWW.youtube.com\n",
			append([]string{"0.0.0.0 m.youtube.com", "0.0.0.0 youtu.be", "0.0.0.0 www.youtu.be"}, v6Lines...)},
		{"commented-out entries don't count",
			"#0.0.0.0 youtube.com\n# 127.0.0.1 youtu.be\n", Lines()},
		{"other names are left alone",
			"127.0.0.1 localhost\n10.0.0.2 nas.local\n0.0.0.0 music.youtube.com notyoutube.com\n", Lines()},
		{"lines that aren't entries are skipped", "not-an-address youtube.com\nyoutube.com\n", Lines()},
	} {
		missing, err := Check([]byte(c.content))
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
		if !slices.Equal(missing, c.missing) {
			t.Errorf("%s: missing\n  %q\nwant\n  %q", c.name, missing, c.missing)
		}
	}
}

func TestRealAddressIsAConflict(t *testing.T) {
	_, err := Check([]byte("0.0.0.0 youtube.com\n142.250.1.1 www.youtube.com youtu.be\n"))
	var conflict *ConflictError
	if !errors.As(err, &conflict) || !slices.Equal(conflict.Entries, []string{"142.250.1.1 www.youtube.com", "142.250.1.1 youtu.be"}) {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(err.Error(), "Remove those lines") {
		t.Errorf("message: %s", err)
	}
}

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestAppendKeepsTheFileAndAddsOnlyWhatsMissing(t *testing.T) {
	before := "127.0.0.1 localhost\n0.0.0.0 youtube.com\n"
	path := writeFile(t, before)
	added, err := Append(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != len(Lines())-1 || slices.Contains(added, "0.0.0.0 youtube.com") {
		t.Errorf("added %q", added)
	}
	after := read(t, path)
	if !strings.HasPrefix(after, before+"\n# Blocked by mindful-yt lock-me-in on ") {
		t.Errorf("the block should follow a blank line after what was there:\n%s", after)
	}
	if strings.Contains(after, "\r") {
		t.Errorf("an LF file got CRLF lines:\n%q", after)
	}
	if missing, err := Check([]byte(after)); err != nil || len(missing) > 0 {
		t.Errorf("still missing %q (%v)", missing, err)
	}

	again, err := Append(path)
	if err != nil || len(again) > 0 || read(t, path) != after {
		t.Errorf("a second run changed the file: added %q, %v", again, err)
	}
}

func TestAppendMatchesCRLFAndEndsTheLastLine(t *testing.T) {
	path := writeFile(t, "# hosts\r\n127.0.0.1 localhost")
	if _, err := Append(path); err != nil {
		t.Fatal(err)
	}
	after := read(t, path)
	if !strings.HasPrefix(after, "# hosts\r\n127.0.0.1 localhost\r\n\r\n# Blocked by") {
		t.Errorf("start: %q", after[:min(len(after), 60)])
	}
	if strings.Count(after, "\n") != strings.Count(after, "\r\n") || !strings.HasSuffix(after, ":: www.youtu.be\r\n") {
		t.Errorf("every line should end in CRLF: %q", after)
	}
}

func TestAppendLeavesAConflictingFileAlone(t *testing.T) {
	before := "8.8.8.8 youtu.be\n"
	path := writeFile(t, before)
	var conflict *ConflictError
	if _, err := Append(path); !errors.As(err, &conflict) {
		t.Fatalf("got %v", err)
	}
	if read(t, path) != before {
		t.Error("the file changed")
	}
}

func TestAppendCreatesAMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts")
	added, err := Append(path)
	if err != nil || !slices.Equal(added, Lines()) {
		t.Fatalf("added %q, %v", added, err)
	}
	if after := read(t, path); !strings.HasPrefix(after, "# Blocked by") {
		t.Errorf("new file: %q", after)
	}
}
