package deps

import (
	"archive/zip"
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

func TestDenoSumReadsBothFormats(t *testing.T) {
	const hash = "a0c3101b4158d1dfb7d6a78a7bf0f3de80c96bb423c152beec8beb22786f2238"
	for name, c := range map[string]struct{ text, want string }{
		// macOS and Linux: sha256sum output.
		"unix": {hash + "  deno-x86_64-unknown-linux-gnu.zip\n", hash},
		// Windows: PowerShell Get-FileHash | Format-List, with CRLF.
		"windows": {"\r\nAlgorithm : SHA256\r\nHash      : " + strings.ToUpper(hash) + "\r\n" +
			`Path      : C:\a\deno\deno\target\release\deno-x86_64-pc-windows-msvc.zip` + "\r\n\r\n", hash},
		"no hash":    {"Not Found", ""},
		"two hashes": {hash + "\n" + strings.Repeat("b", 64) + "\n", ""},
	} {
		if got := denoSum(c.text); got != c.want {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
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
		ytdlp, deno  string
	}{
		{"windows", "amd64", false, "yt-dlp.exe", "deno-x86_64-pc-windows-msvc.zip"},
		{"windows", "arm64", false, "yt-dlp_arm64.exe", "deno-aarch64-pc-windows-msvc.zip"},
		{"darwin", "arm64", false, "yt-dlp_macos", "deno-aarch64-apple-darwin.zip"},
		{"darwin", "amd64", false, "yt-dlp_macos", "deno-x86_64-apple-darwin.zip"},
		{"linux", "amd64", false, "yt-dlp_linux", "deno-x86_64-unknown-linux-gnu.zip"},
		{"linux", "arm64", false, "yt-dlp_linux_aarch64", "deno-aarch64-unknown-linux-gnu.zip"},
		{"linux", "amd64", true, "yt-dlp_musllinux", "deno-x86_64-unknown-linux-gnu.zip"},
		{"freebsd", "amd64", false, "", ""},
	} {
		if got, _ := ytdlpAsset(c.goos, c.goarch, c.musl); got != c.ytdlp {
			t.Errorf("yt-dlp for %s/%s musl=%v: got %q, want %q", c.goos, c.goarch, c.musl, got, c.ytdlp)
		}
		if got, _ := denoAsset(c.goos, c.goarch); got != c.deno {
			t.Errorf("deno for %s/%s: got %q, want %q", c.goos, c.goarch, got, c.deno)
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

func TestExtractFindsTheProgram(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "deno.zip")
	f, _ := os.Create(archive)
	w := zip.NewWriter(f)
	for name, body := range map[string]string{"README.md": "readme", "deno": "the program"} {
		entry, _ := w.Create(name)
		entry.Write([]byte(body))
	}
	w.Close()
	f.Close()

	out, err := extract(archive, "deno", dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(out); string(got) != "the program" {
		t.Errorf("extracted %q", got)
	}
	if _, err := extract(archive, "deno.exe", dir); err == nil {
		t.Error("missing entry wasn't reported")
	}
}

func TestJSRuntimeVersionRules(t *testing.T) {
	node, deno := jsRuntimes[0], jsRuntimes[1]
	for _, c := range []struct {
		rt     jsRuntime
		output string
		want   string
	}{
		{node, "v24.17.0\n", "node 24.17"},
		{node, "v22.0.0\n", "node 22.0"},
		{node, "v20.11.1\n", ""}, // older than yt-dlp's minimum
		{deno, "deno 2.9.6 (stable, release, x86_64-pc-windows-msvc)\n", "deno 2.9"},
		{deno, "deno 2.2.0\n", ""},
		{node, "not a version", ""},
	} {
		if got, _ := c.rt.check(c.output); got != c.want {
			t.Errorf("%s %q: got %q, want %q", c.rt.name, c.output, got, c.want)
		}
	}
}
