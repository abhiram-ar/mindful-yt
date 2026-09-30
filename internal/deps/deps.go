// Package deps finds the programs mindful-yt relies on (yt-dlp, a JavaScript
// runtime and ffmpeg) and knows how to install each one on this OS.
package deps

import (
	"context"
	"debug/elf"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// Dependency is a program mindful-yt needs.
type Dependency struct {
	Name   string
	Why    string
	Manual string // how to get it by hand
	// How mindful-yt installs it on this OS. At most one is set; neither means by hand.
	Download func(ctx context.Context, progress func(done, total int64)) error // mindful-yt fetches it itself
	Command  []string                                                          // an installer to run in the terminal
}

// YtdlpPath is where mindful-yt keeps its own yt-dlp.
func YtdlpPath(tools string) string { return filepath.Join(tools, exe("yt-dlp")) }

// DenoPath is where mindful-yt keeps Deno when it installs it. yt-dlp finds it
// there because ytdlp.Command puts the tools folder first on yt-dlp's PATH.
func DenoPath(tools string) string { return filepath.Join(tools, exe("deno")) }

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// Missing lists what isn't installed. tools is mindful-yt's tools folder.
func Missing(tools string) []Dependency {
	var missing []Dependency
	if _, err := os.Stat(YtdlpPath(tools)); err != nil {
		missing = append(missing, ytdlpDependency(tools))
	}
	if FindJSRuntime(tools) == "" {
		missing = append(missing, jsDependency(tools))
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		missing = append(missing, ffmpegDependency(runtime.GOOS, exec.LookPath, os.Geteuid() == 0))
	}
	return missing
}

func ytdlpDependency(tools string) Dependency {
	d := Dependency{
		Name: "yt-dlp", Why: "it's the downloader itself",
		Manual: "https://github.com/yt-dlp/yt-dlp/releases",
	}
	if _, ok := ytdlpAsset(runtime.GOOS, runtime.GOARCH, isMusl()); ok {
		d.Download = func(ctx context.Context, progress func(done, total int64)) error {
			return InstallYtdlp(ctx, YtdlpPath(tools), progress)
		}
	}
	return d
}

func jsDependency(tools string) Dependency {
	d := Dependency{
		Name:   "Deno",
		Why:    "yt-dlp runs YouTube's player code with it to unlock the videos (Node.js 22+ works too)",
		Manual: "https://deno.com",
	}
	if _, ok := denoAsset(runtime.GOOS, runtime.GOARCH); ok && !isMusl() {
		d.Download = func(ctx context.Context, progress func(done, total int64)) error {
			return InstallDeno(ctx, DenoPath(tools), progress)
		}
	}
	return d
}

func ffmpegDependency(goos string, lookPath func(string) (string, error), root bool) Dependency {
	d := Dependency{
		Name: "ffmpeg", Why: "it joins the separate video and audio streams into one file",
		Command: ffmpegInstallCommand(goos, lookPath, root),
	}
	switch goos {
	case "windows":
		d.Manual = "https://github.com/yt-dlp/FFmpeg-Builds"
	case "darwin":
		d.Manual = "install Homebrew (https://brew.sh), then run: brew install ffmpeg"
	default:
		d.Manual = "install ffmpeg with your distribution's package manager"
	}
	return d
}

// Linux package managers, in the order they're looked for.
var linuxPackageManagers = []struct {
	name    string
	command []string
}{
	// A fresh Debian or Ubuntu may have no package lists yet.
	{"apt-get", []string{"sh", "-c", "apt-get update && apt-get install -y ffmpeg"}},
	{"dnf", []string{"dnf", "install", "-y", "ffmpeg-free"}},
	{"pacman", []string{"pacman", "-S", "--needed", "--noconfirm", "ffmpeg"}},
	{"zypper", []string{"zypper", "--non-interactive", "install", "ffmpeg"}},
	{"apk", []string{"apk", "add", "ffmpeg"}},
}

// ffmpegInstallCommand picks this OS's package manager, or returns nil when
// there's none mindful-yt knows how to use.
func ffmpegInstallCommand(goos string, lookPath func(string) (string, error), root bool) []string {
	has := func(name string) bool { _, err := lookPath(name); return err == nil }
	switch goos {
	case "windows":
		if has("winget") {
			return []string{"winget", "install", "--id", "yt-dlp.FFmpeg", "--exact", "--source", "winget"}
		}
	case "darwin":
		if has("brew") {
			return []string{"brew", "install", "ffmpeg"}
		}
	case "linux":
		for _, pm := range linuxPackageManagers {
			switch {
			case !has(pm.name):
				continue
			case root:
				return pm.command
			case has("sudo"):
				return append([]string{"sudo"}, pm.command...)
			}
			return nil
		}
	}
	return nil
}

// CommandLine shows an installer command the way you'd type it.
func CommandLine(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " &|;") {
			a = `"` + a + `"`
		}
		quoted[i] = a
	}
	return strings.Join(quoted, " ")
}

type jsRuntime struct {
	name    string
	version *regexp.Regexp
	min     [2]int
}

// yt-dlp's own minimums, from yt_dlp/utils/_jsruntime.py
var jsRuntimes = []jsRuntime{
	{"node", regexp.MustCompile(`v(\d+)\.(\d+)`), [2]int{22, 0}},
	{"deno", regexp.MustCompile(`deno (\d+)\.(\d+)`), [2]int{2, 3}},
}

// FindJSRuntime returns e.g. "node 24.17" for a runtime yt-dlp can use, on
// PATH or in mindful-yt's tools folder, or "" if there's none.
func FindJSRuntime(tools string) string {
	for _, rt := range jsRuntimes {
		var candidates []string
		if path, err := exec.LookPath(rt.name); err == nil {
			candidates = append(candidates, path)
		}
		if rt.name == "deno" {
			candidates = append(candidates, DenoPath(tools))
		}
		for _, path := range candidates {
			out, err := exec.Command(path, "--version").Output()
			if err != nil {
				continue
			}
			if found, ok := rt.check(string(out)); ok {
				return found
			}
		}
	}
	return ""
}

func (rt jsRuntime) check(versionOutput string) (string, bool) {
	m := rt.version.FindStringSubmatch(versionOutput)
	if m == nil {
		return "", false
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	if major < rt.min[0] || major == rt.min[0] && minor < rt.min[1] {
		return "", false
	}
	return fmt.Sprintf("%s %d.%d", rt.name, major, minor), true
}

// isMusl reports whether this is a musl Linux such as Alpine, which needs its
// own yt-dlp build and has no official Deno build. It asks which loader the
// system's own /bin/sh uses, because glibc systems can have the musl package
// (and its /lib/ld-musl-* loader) installed alongside.
func isMusl() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if loader, err := elfInterpreter("/bin/sh"); err == nil && loader != "" {
		return strings.Contains(loader, "ld-musl")
	}
	loaders, _ := filepath.Glob("/lib/ld-musl-*") // a static /bin/sh can't tell us
	return len(loaders) > 0
}

// elfInterpreter returns the dynamic loader a program asks for, or "" if it
// has none.
func elfInterpreter(path string) (string, error) {
	f, err := elf.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	for _, p := range f.Progs {
		if p.Type == elf.PT_INTERP {
			data, err := io.ReadAll(p.Open())
			if err != nil {
				return "", err
			}
			return strings.TrimRight(string(data), "\x00"), nil
		}
	}
	return "", nil
}

// Summary describes what was found, for --check.
func Summary(tools string) string {
	version := "?"
	if out, err := exec.Command(YtdlpPath(tools), "--version").Output(); err == nil {
		version = strings.TrimSpace(string(out))
	}
	ffmpeg, _ := exec.LookPath("ffmpeg")
	return fmt.Sprintf("yt-dlp   %s  %s\nJS       %s\nffmpeg   %s",
		version, YtdlpPath(tools), FindJSRuntime(tools), ffmpeg)
}
