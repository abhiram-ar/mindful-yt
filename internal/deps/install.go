package deps

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const (
	ytdlpRelease = "https://github.com/yt-dlp/yt-dlp/releases/latest/download/"
	denoRelease  = "https://github.com/denoland/deno/releases/latest/download/"
)

// ytdlpAsset is the standalone yt-dlp release file for an OS and architecture.
func ytdlpAsset(goos, goarch string, musl bool) (string, bool) {
	switch goos + "/" + goarch {
	case "windows/amd64":
		return "yt-dlp.exe", true
	case "windows/arm64":
		return "yt-dlp_arm64.exe", true
	case "windows/386":
		return "yt-dlp_x86.exe", true
	case "darwin/amd64", "darwin/arm64":
		return "yt-dlp_macos", true // one universal build
	case "linux/amd64":
		if musl {
			return "yt-dlp_musllinux", true
		}
		return "yt-dlp_linux", true
	case "linux/arm64":
		if musl {
			return "yt-dlp_musllinux_aarch64", true
		}
		return "yt-dlp_linux_aarch64", true
	}
	return "", false
}

// denoAsset is the Deno release zip for an OS and architecture. Deno builds
// Linux for glibc only.
func denoAsset(goos, goarch string) (string, bool) {
	arch, ok := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[goarch]
	if !ok {
		return "", false
	}
	target, ok := map[string]string{
		"windows": "pc-windows-msvc", "darwin": "apple-darwin", "linux": "unknown-linux-gnu",
	}[goos]
	if !ok {
		return "", false
	}
	return "deno-" + arch + "-" + target + ".zip", true
}

// InstallYtdlp downloads the official yt-dlp build for this OS to dest,
// checking it against the release's published SHA-256 sums.
func InstallYtdlp(ctx context.Context, dest string, progress func(done, total int64)) error {
	asset, ok := ytdlpAsset(runtime.GOOS, runtime.GOARCH, isMusl())
	if !ok {
		return fmt.Errorf("yt-dlp has no standalone build for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	want, err := fetchSum(ctx, ytdlpRelease+"SHA2-256SUMS", asset,
		func(sums string) string { return expectedSum(sums, asset) })
	if err != nil {
		return err
	}
	tmp, err := downloadVerified(ctx, ytdlpRelease+asset, filepath.Dir(dest), want, progress)
	if err != nil {
		return err
	}
	defer os.Remove(tmp) // no-op once moved into place
	return install(tmp, dest)
}

// InstallDeno downloads the official Deno build for this OS, checks it
// against its published SHA-256 sum, and unpacks it to dest.
func InstallDeno(ctx context.Context, dest string, progress func(done, total int64)) error {
	asset, ok := denoAsset(runtime.GOOS, runtime.GOARCH)
	if !ok || isMusl() {
		return fmt.Errorf("Deno has no build for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	want, err := fetchSum(ctx, denoRelease+asset+".sha256sum", asset, denoSum)
	if err != nil {
		return err
	}
	archive, err := downloadVerified(ctx, denoRelease+asset, filepath.Dir(dest), want, progress)
	if err != nil {
		return err
	}
	defer os.Remove(archive)
	tmp, err := extract(archive, filepath.Base(dest), filepath.Dir(dest))
	if err != nil {
		return err
	}
	defer os.Remove(tmp) // no-op once moved into place
	return install(tmp, dest)
}

// install makes a downloaded program executable and moves it into place.
func install(tmp, dest string) error {
	if err := os.Chmod(tmp, 0o755); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

// fetchSum downloads a checksum file and reads name's SHA-256 out of it with parse.
func fetchSum(ctx context.Context, url, name string, parse func(string) string) (string, error) {
	resp, err := httpGet(ctx, url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	sums, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	want := parse(string(sums))
	if want == "" {
		return "", fmt.Errorf("%s isn't listed in %s", name, url)
	}
	return want, nil
}

var sha256Hex = regexp.MustCompile(`(?i)\b[0-9a-f]{64}\b`)

// denoSum reads the hash out of one of Deno's per-file .sha256sum files. They
// hold "<hash>  <name>" for macOS and Linux, but PowerShell Get-FileHash
// output ("Hash : <HEX>" and a "Path : ..." line) for Windows. Anything but
// exactly one hash is refused rather than guessed at.
func denoSum(text string) string {
	hashes := sha256Hex.FindAllString(text, -1)
	if len(hashes) != 1 {
		return ""
	}
	return strings.ToLower(hashes[0])
}

// expectedSum finds name in sha256sum-style lines ("<hash>  <name>").
func expectedSum(sums, name string) string {
	for _, line := range strings.Split(sums, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			return strings.ToLower(f[0])
		}
	}
	return ""
}

// downloadVerified saves url to a temporary file in dir and returns its path,
// or an error if it doesn't match the SHA-256 sum want.
func downloadVerified(ctx context.Context, url, dir, want string, progress func(done, total int64)) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	// A download that was cancelled when ytget quit can leave its temp file behind.
	for _, pattern := range []string{"download-*.part", "extract-*.part"} {
		stale, _ := filepath.Glob(filepath.Join(dir, pattern))
		for _, f := range stale {
			os.Remove(f)
		}
	}
	resp, err := httpGet(ctx, url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	f, err := os.CreateTemp(dir, "download-*.part")
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	counter := &progressWriter{total: resp.ContentLength, report: progress}
	_, err = io.Copy(io.MultiWriter(f, hash, counter), resp.Body)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if got := hex.EncodeToString(hash.Sum(nil)); err == nil && got != want {
		err = fmt.Errorf("%s failed its checksum (got %s, expected %s)", path.Base(url), got, want)
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// extract copies the file called name out of a zip archive into a temporary
// file in dir and returns its path.
func extract(archive, name, dir string) (string, error) {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return "", err
	}
	defer r.Close()
	for _, entry := range r.File {
		if path.Base(entry.Name) != name || entry.FileInfo().IsDir() {
			continue
		}
		src, err := entry.Open()
		if err != nil {
			return "", err
		}
		defer src.Close()
		out, err := os.CreateTemp(dir, "extract-*.part")
		if err != nil {
			return "", err
		}
		_, err = io.Copy(out, src)
		if closeErr := out.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			os.Remove(out.Name())
			return "", err
		}
		return out.Name(), nil
	}
	return "", fmt.Errorf("%s isn't in %s", name, filepath.Base(archive))
}

func httpGet(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	return resp, nil
}

type progressWriter struct {
	done, total int64
	report      func(done, total int64)
}

func (w *progressWriter) Write(p []byte) (int, error) {
	w.done += int64(len(p))
	if w.report != nil {
		w.report(w.done, w.total)
	}
	return len(p), nil
}
