package deps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const ytdlpRelease = "https://github.com/yt-dlp/yt-dlp/releases/latest/download/"

// InstallYtdlp downloads the official yt-dlp.exe to dest, checking it against
// the release's published SHA-256 sums before putting it in place.
func InstallYtdlp(ctx context.Context, dest string, progress func(done, total int64)) error {
	resp, err := httpGet(ctx, ytdlpRelease+"SHA2-256SUMS")
	if err != nil {
		return err
	}
	sums, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if err != nil {
		return err
	}
	want := expectedSum(string(sums), "yt-dlp.exe")
	if want == "" {
		return errors.New("yt-dlp.exe isn't listed in the release checksums")
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	resp, err = httpGet(ctx, ytdlpRelease+"yt-dlp.exe")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dest), "yt-dlp-*.part")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once renamed

	hash := sha256.New()
	counter := &progressWriter{total: resp.ContentLength, report: progress}
	_, err = io.Copy(io.MultiWriter(tmp, hash, counter), resp.Body)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != want {
		return fmt.Errorf("yt-dlp.exe failed its checksum (got %s, expected %s)", got, want)
	}
	return os.Rename(tmp.Name(), dest)
}

func expectedSum(sums, name string) string {
	for _, line := range strings.Split(sums, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			return strings.ToLower(f[0])
		}
	}
	return ""
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
