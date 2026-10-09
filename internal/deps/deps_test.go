package deps

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestExpectedSum(t *testing.T) {
	sums := "aa11  yt-dlp\nBB22  yt-dlp.exe\ncc33  yt-dlp_x86.exe\n"
	if got := expectedSum(sums, "yt-dlp.exe"); got != "bb22" {
		t.Errorf("got %q", got)
	}
	if got := expectedSum(sums, "missing.exe"); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestNewestLTSSkipsCurrentReleases(t *testing.T) {
	// The shape of https://nodejs.org/dist/index.json, newest first.
	const index = `[
		{"version":"v26.11.1","date":"2026-10-01","files":["linux-x64"],"lts":false},
		{"version":"v24.21.0","date":"2026-09-08","files":["linux-x64"],"lts":"Krypton"},
		{"version":"v22.30.0","date":"2026-09-01","files":["linux-x64"],"lts":"Jod"}
	]`
	if got, err := newestLTS(strings.NewReader(index)); err != nil || got != "v24.21.0" {
		t.Errorf("got %q, %v", got, err)
	}
	for name, bad := range map[string]string{
		"no LTS":      `[{"version":"v26.11.1","lts":false}]`,
		"odd version": `[{"version":"../../evil","lts":"Krypton"}]`,
		"not a list":  `{"version":"v24.21.0","lts":"Krypton"}`,
		"cut short":   `[{"version":"v26.11.1","lts":false},{"vers`,
	} {
		if got, err := newestLTS(strings.NewReader(bad)); err == nil {
			t.Errorf("%s: got %q, want an error", name, got)
		}
	}
}

func TestMuslDetectionUsesTheSystemLoader(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}
	loader, err := elfInterpreter("/bin/sh")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("/bin/sh loads with %q", loader)
	if want := strings.Contains(loader, "ld-musl"); loader != "" && isMusl() != want {
		t.Errorf("isMusl() = %v with loader %q", isMusl(), loader)
	}
}

func TestReleaseAssetsPerPlatform(t *testing.T) {
	for _, c := range []struct {
		goos, goarch string
		musl         bool
		ytdlp, node  string
	}{
		{"windows", "amd64", false, "yt-dlp.exe", "win-x64"},
		{"windows", "arm64", false, "yt-dlp_arm64.exe", "win-arm64"},
		{"windows", "386", false, "yt-dlp_x86.exe", ""},
		{"darwin", "arm64", false, "yt-dlp_macos", "darwin-arm64"},
		{"darwin", "amd64", false, "yt-dlp_macos", "darwin-x64"},
		{"linux", "amd64", false, "yt-dlp_linux", "linux-x64"},
		{"linux", "arm64", false, "yt-dlp_linux_aarch64", "linux-arm64"},
		{"linux", "amd64", true, "yt-dlp_musllinux", "linux-x64"}, // InstallNode refuses musl
		{"freebsd", "amd64", false, "", ""},
	} {
		if got, _ := ytdlpAsset(c.goos, c.goarch, c.musl); got != c.ytdlp {
			t.Errorf("yt-dlp for %s/%s musl=%v: got %q, want %q", c.goos, c.goarch, c.musl, got, c.ytdlp)
		}
		if got, _ := nodePlatform(c.goos, c.goarch); got != c.node {
			t.Errorf("node for %s/%s: got %q, want %q", c.goos, c.goarch, got, c.node)
		}
	}
}

func TestNodeArchiveNames(t *testing.T) {
	// As listed in https://nodejs.org/dist/v24.21.0/SHASUMS256.txt.
	for _, c := range []struct{ platform, archive, program string }{
		{"win-x64", "node-v24.21.0-win-x64.zip", "node-v24.21.0-win-x64/node.exe"},
		{"darwin-arm64", "node-v24.21.0-darwin-arm64.tar.gz", "node-v24.21.0-darwin-arm64/bin/node"},
		{"linux-x64", "node-v24.21.0-linux-x64.tar.gz", "node-v24.21.0-linux-x64/bin/node"},
	} {
		if archive, program := nodeArchive("v24.21.0", c.platform); archive != c.archive || program != c.program {
			t.Errorf("%s: got %q and %q", c.platform, archive, program)
		}
	}
}

func TestFFmpegInstallerPerOS(t *testing.T) {
	onPath := func(names ...string) func(string) (string, error) {
		return func(name string) (string, error) {
			if slices.Contains(names, name) {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("not found")
		}
	}
	for _, c := range []struct {
		name   string
		goos   string
		lookup func(string) (string, error)
		root   bool
		want   []string
	}{
		{"windows", "windows", onPath("winget"), false,
			[]string{"winget", "install", "--id", "yt-dlp.FFmpeg", "--exact", "--source", "winget"}},
		{"windows without winget", "windows", onPath(), false, nil},
		{"mac with brew", "darwin", onPath("brew"), false, []string{"brew", "install", "ffmpeg"}},
		{"mac without brew", "darwin", onPath(), false, nil},
		{"ubuntu", "linux", onPath("apt-get", "sudo"), false,
			[]string{"sudo", "sh", "-c", "apt-get update && apt-get install -y ffmpeg"}},
		{"fedora", "linux", onPath("dnf", "sudo"), false, []string{"sudo", "dnf", "install", "-y", "ffmpeg-free"}},
		{"arch as root", "linux", onPath("pacman"), true, []string{"pacman", "-S", "--needed", "--noconfirm", "ffmpeg"}},
		{"alpine without sudo", "linux", onPath("apk"), false, nil},
		{"unknown linux", "linux", onPath("sudo"), false, nil},
	} {
		if got := ffmpegInstallCommand(c.goos, c.lookup, c.root); !slices.Equal(got, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestCommandLineQuotesArguments(t *testing.T) {
	got := CommandLine([]string{"sudo", "sh", "-c", "apt-get update && apt-get install -y ffmpeg"})
	if want := `sudo sh -c "apt-get update && apt-get install -y ffmpeg"`; got != want {
		t.Errorf("got %s", got)
	}
}

func TestUnzipFindsTheProgram(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "node.zip")
	f, _ := os.Create(archive)
	w := zip.NewWriter(f)
	for name, body := range map[string]string{
		"node-v1/README.md": "readme", "node-v1/node_modules/x/node.exe": "not this one",
		"node-v1/node.exe": "the program",
	} {
		entry, _ := w.Create(name)
		entry.Write([]byte(body))
	}
	w.Close()
	f.Close()

	out, err := unzip(archive, "node-v1/node.exe", dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(out); string(got) != "the program" {
		t.Errorf("extracted %q", got)
	}
	if _, err := unzip(archive, "node-v1/bin/node.exe", dir); err == nil {
		t.Error("missing entry wasn't reported")
	}
}

func TestUntarGzFindsTheProgram(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "node.tar.gz")
	f, _ := os.Create(archive)
	gz := gzip.NewWriter(f)
	w := tar.NewWriter(gz)
	add := func(hdr *tar.Header, body string) {
		hdr.Mode, hdr.Size = 0o755, int64(len(body))
		w.WriteHeader(hdr)
		w.Write([]byte(body))
	}
	add(&tar.Header{Name: "node-v1/bin/", Typeflag: tar.TypeDir}, "")
	add(&tar.Header{Name: "node-v1/bin/npm", Typeflag: tar.TypeSymlink, Linkname: "../lib/npm-cli.js"}, "")
	add(&tar.Header{Name: "node-v1/lib/bin/node", Typeflag: tar.TypeReg}, "not this one")
	add(&tar.Header{Name: "node-v1/bin/node", Typeflag: tar.TypeReg}, "the program")
	add(&tar.Header{Name: "node-v1/bin/link", Typeflag: tar.TypeSymlink, Linkname: "node"}, "")
	w.Close()
	gz.Close()
	f.Close()

	out, err := untarGz(archive, "node-v1/bin/node", dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(out); string(got) != "the program" {
		t.Errorf("extracted %q", got)
	}
	for _, name := range []string{"node-v1/bin/npm", "node-v1/bin/link", "node-v1/bin/node.exe"} {
		if _, err := untarGz(archive, name, dir); err == nil {
			t.Errorf("%s: a link or missing entry wasn't refused", name)
		}
	}
}

func TestJSRuntimeVersionRules(t *testing.T) {
	node := jsRuntimes[0]
	for _, c := range []struct {
		rt     jsRuntime
		output string
		want   string
	}{
		{node, "v24.17.0\n", "node 24.17"},
		{node, "v22.0.0\n", "node 22.0"},
		{node, "v20.11.1\n", ""}, // older than yt-dlp's minimum
		{node, "not a version", ""},
	} {
		if got, _ := c.rt.check(c.output); got != c.want {
			t.Errorf("%s %q: got %q, want %q", c.rt.name, c.output, got, c.want)
		}
	}
}
