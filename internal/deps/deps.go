// Package deps finds the programs ytget relies on (yt-dlp, a JavaScript
// runtime and ffmpeg) and installs the ones that are missing.
package deps

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// Dependency is a program ytget needs.
type Dependency struct {
	Name   string
	Why    string
	Manual string // where to get it by hand
	Winget string // winget package id; empty for yt-dlp, which ytget downloads itself
}

// The programs ytget checks for.
var (
	Ytdlp = Dependency{
		Name: "yt-dlp", Why: "it's the downloader itself",
		Manual: "https://github.com/yt-dlp/yt-dlp/releases",
	}
	JSRuntime = Dependency{
		Name: "Node.js 22 or newer", Why: "yt-dlp runs YouTube's player code with it to unlock the videos",
		Manual: "https://nodejs.org", Winget: "OpenJS.NodeJS.LTS",
	}
	FFmpeg = Dependency{
		Name: "ffmpeg", Why: "it joins the separate video and audio streams into one file",
		Manual: "https://github.com/yt-dlp/FFmpeg-Builds", Winget: "yt-dlp.FFmpeg",
	}
)

// Missing lists what isn't installed. ytdlpPath is ytget's own copy of yt-dlp.exe.
func Missing(ytdlpPath string) []Dependency {
	var missing []Dependency
	if _, err := os.Stat(ytdlpPath); err != nil {
		missing = append(missing, Ytdlp)
	}
	if FindJSRuntime() == "" {
		missing = append(missing, JSRuntime)
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		missing = append(missing, FFmpeg)
	}
	return missing
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

// FindJSRuntime returns e.g. "node 24.17" for a runtime yt-dlp can use, or "".
func FindJSRuntime() string {
	for _, rt := range jsRuntimes {
		path, err := exec.LookPath(rt.name)
		if err != nil {
			continue
		}
		out, err := exec.Command(path, "--version").Output()
		if err != nil {
			continue
		}
		if found, ok := rt.check(string(out)); ok {
			return found
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

func WingetArgs(id string) []string {
	return []string{"install", "--id", id, "--exact", "--source", "winget"}
}

// Summary describes what was found, for --check.
func Summary(ytdlpPath string) string {
	version := "?"
	if out, err := exec.Command(ytdlpPath, "--version").Output(); err == nil {
		version = strings.TrimSpace(string(out))
	}
	ffmpeg, _ := exec.LookPath("ffmpeg")
	return fmt.Sprintf("yt-dlp   %s  %s\nJS       %s\nffmpeg   %s", version, ytdlpPath, FindJSRuntime(), ffmpeg)
}
