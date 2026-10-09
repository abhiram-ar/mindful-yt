package deps

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	nodeDist     = "https://nodejs.org/dist/"
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

// nodePlatform is Node.js's name for an OS and architecture, as its release
// files use it. On Linux it's the glibc build.
func nodePlatform(goos, goarch string) (string, bool) {
	arch, ok := map[string]string{"amd64": "x64", "arm64": "arm64"}[goarch]
	if !ok {
		return "", false
	}
	system, ok := map[string]string{"windows": "win", "darwin": "darwin", "linux": "linux"}[goos]
	if !ok {
		return "", false
	}
	return system + "-" + arch, true
}

// nodeArchive is the release file of a Node.js version for a platform, and the
// path of the node program inside it.
func nodeArchive(version, platform string) (archive, program string) {
	dir := "node-" + version + "-" + platform
	if strings.HasPrefix(platform, "win-") {
		return dir + ".zip", dir + "/node.exe"
	}
	return dir + ".tar.gz", dir + "/bin/node"
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

// InstallNode downloads the official build of the newest Node.js LTS for this
// OS, checks it against the release's published SHA-256 sums, and unpacks
// node to dest.
func InstallNode(ctx context.Context, dest string, progress func(done, total int64)) error {
	platform, ok := nodePlatform(runtime.GOOS, runtime.GOARCH)
	if !ok || isMusl() {
		return fmt.Errorf("Node.js has no build for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	version, err := nodeLTS(ctx)
	if err != nil {
		return err
	}
	asset, program := nodeArchive(version, platform)
	release := nodeDist + version + "/"
	want, err := fetchSum(ctx, release+"SHASUMS256.txt", asset,
		func(sums string) string { return expectedSum(sums, asset) })
	if err != nil {
		return err
	}
	archive, err := downloadVerified(ctx, release+asset, filepath.Dir(dest), want, progress)
	if err != nil {
		return err
	}
	defer os.Remove(archive)
	unpack := untarGz
	if strings.HasSuffix(asset, ".zip") {
		unpack = unzip
	}
	tmp, err := unpack(archive, program, filepath.Dir(dest))
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

// nodeLTS asks nodejs.org for the newest LTS version of Node.js, e.g. "v24.21.0".
func nodeLTS(ctx context.Context) (string, error) {
	resp, err := httpGet(ctx, nodeDist+"index.json")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	return newestLTS(io.LimitReader(resp.Body, 16<<20))
}

var nodeVersion = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// newestLTS reads Node.js's release list, which is newest first, only as far
// as the first LTS release.
func newestLTS(index io.Reader) (string, error) {
	bad := func(err error) (string, error) {
		return "", fmt.Errorf("couldn't read Node.js's release list: %w", err)
	}
	dec := json.NewDecoder(index)
	if tok, err := dec.Token(); err != nil {
		return bad(err)
	} else if tok != json.Delim('[') {
		return bad(errors.New("it isn't a list"))
	}
	for dec.More() {
		var release struct {
			Version string `json:"version"`
			LTS     any    `json:"lts"` // false, or the LTS line's name
		}
		if err := dec.Decode(&release); err != nil {
			return bad(err)
		}
		if _, ok := release.LTS.(string); !ok {
			continue
		}
		if !nodeVersion.MatchString(release.Version) {
			return bad(fmt.Errorf("odd version %q", release.Version))
		}
		return release.Version, nil
	}
	return "", errors.New("Node.js's release list has no LTS release")
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
	// A download that was cancelled when mindful-yt quit can leave its temp file behind.
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

// unzip copies the file at path name in a zip archive into a temporary file
// in dir and returns its path.
func unzip(archive, name, dir string) (string, error) {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return "", err
	}
	defer r.Close()
	for _, entry := range r.File {
		if entry.Name != name || entry.FileInfo().IsDir() {
			continue
		}
		src, err := entry.Open()
		if err != nil {
			return "", err
		}
		defer src.Close()
		return saveTemp(src, dir)
	}
	return "", fmt.Errorf("%s isn't in the archive", name)
}

// untarGz does the same for a .tar.gz archive. Only a regular file counts, not
// a link.
func untarGz(archive, name, dir string) (string, error) {
	f, err := os.Open(archive)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	r := tar.NewReader(gz)
	for {
		entry, err := r.Next()
		if err == io.EOF {
			return "", fmt.Errorf("%s isn't in the archive", name)
		}
		if err != nil {
			return "", err
		}
		if entry.Name == name && entry.Typeflag == tar.TypeReg {
			return saveTemp(r, dir)
		}
	}
}

// saveTemp copies src into a temporary file in dir and returns its path.
func saveTemp(src io.Reader, dir string) (string, error) {
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
