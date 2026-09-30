// Package hosts blocks YouTube in the operating system's hosts file, for
// mindful-yt lock-me-in. It only ever adds lines: what's already in the file
// stays as it is.
package hosts

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"net/netip"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Domains are the names lock-me-in blocks. A hosts file can't block every
// subdomain at once, so these are the ones people open.
var Domains = []string{"youtube.com", "www.youtube.com", "m.youtube.com", "youtu.be", "www.youtu.be"}

// Each domain gets an IPv4 and an IPv6 line. With only the IPv4 one, a
// browser on an IPv6 network can still look up YouTube's IPv6 address.
var blockAddrs = []netip.Addr{netip.IPv4Unspecified(), netip.IPv6Unspecified()}

// Lines are the lines a fully blocked hosts file has, or an equivalent of.
func Lines() []string {
	var lines []string
	for _, addr := range blockAddrs {
		for _, domain := range Domains {
			lines = append(lines, addr.String()+" "+domain)
		}
	}
	return lines
}

// ConflictError means the hosts file sends a blocked domain to a real
// address. A block line added after it can't override that.
type ConflictError struct {
	Entries []string // e.g. "1.2.3.4 youtube.com"
}

func (e *ConflictError) Error() string {
	those := "that line"
	if len(e.Entries) > 1 {
		those = "those lines"
	}
	return "the hosts file points YouTube at a real address (" + strings.Join(e.Entries, ", ") +
		"), which a block can't override. Remove " + those + ", then run mindful-yt lock-me-in again"
}

// Check returns the block lines a hosts file's content is missing. A domain
// counts as blocked for IPv4 or IPv6 when it maps to a loopback or unspecified
// address there, such as 127.0.0.1 or ::1, so a file blocked by hand counts
// too. The error is a *ConflictError when a domain points somewhere real.
func Check(content []byte) ([]string, error) {
	type family struct {
		domain string
		v6     bool
	}
	wanted := map[string]bool{}
	for _, d := range Domains {
		wanted[d] = true
	}
	blocked := map[family]bool{}
	var conflicts []string
	for _, line := range strings.Split(string(bytes.TrimPrefix(content, []byte("\xef\xbb\xbf"))), "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		addr, err := netip.ParseAddr(fields[0])
		if err != nil {
			continue
		}
		addr = addr.Unmap()
		for _, name := range fields[1:] {
			name = strings.TrimSuffix(strings.ToLower(name), ".")
			switch {
			case !wanted[name]:
			case addr.IsLoopback() || addr.IsUnspecified():
				blocked[family{name, addr.Is6()}] = true
			default:
				conflicts = append(conflicts, fields[0]+" "+name)
			}
		}
	}
	if len(conflicts) > 0 {
		return nil, &ConflictError{conflicts}
	}

	var missing []string
	for _, addr := range blockAddrs {
		for _, domain := range Domains {
			if !blocked[family{domain, addr.Is6()}] {
				missing = append(missing, addr.String()+" "+domain)
			}
		}
	}
	return missing, nil
}

// Append adds the block lines the hosts file at path is missing, under a
// comment saying where they came from, and returns them. It only appends, so
// the file keeps its permissions and everything already in it. A missing file
// is created.
func Append(path string) ([]string, error) {
	content, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	missing, err := Check(content)
	if err != nil || len(missing) == 0 {
		return nil, err
	}

	nl := "\n"
	if bytes.Contains(content, []byte("\r\n")) || (len(content) == 0 && runtime.GOOS == "windows") {
		nl = "\r\n"
	}
	var b strings.Builder
	if len(content) > 0 {
		if !bytes.HasSuffix(content, []byte("\n")) {
			b.WriteString(nl)
		}
		if !bytes.HasSuffix(content, []byte("\n\n")) && !bytes.HasSuffix(content, []byte("\n\r\n")) {
			b.WriteString(nl) // a blank line before the block
		}
	}
	b.WriteString("# Blocked by mindful-yt lock-me-in on " + time.Now().Format("2006-01-02") + nl)
	for _, line := range missing {
		b.WriteString(line + nl)
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	if _, err := f.WriteString(b.String()); err != nil {
		f.Close()
		return nil, err
	}
	return missing, f.Close()
}

// flush runs a command that clears a DNS cache. Whether it works doesn't
// matter much: the block is in place either way, and caches expire.
func flush(name string, args ...string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	exec.CommandContext(ctx, name, args...).Run()
}
