//go:build unix

package platform

import (
	"errors"
	"io"
	"os"
	"os/exec"
)

// IsAdmin reports whether mindful-yt runs as root.
func IsAdmin() bool { return os.Geteuid() == 0 }

// AdminCommand runs exe as root through sudo, or doas where there's no sudo.
// Either may ask for a password in the terminal. exe should be an absolute
// path: sudo searches its own PATH, which doesn't have ~/.local/bin.
func AdminCommand(exe string, args ...string) (Command, error) {
	return adminCommand(exec.LookPath, exe, args...)
}

func adminCommand(lookPath func(string) (string, error), exe string, args ...string) (Command, error) {
	for _, tool := range []string{"sudo", "doas"} {
		if path, err := lookPath(tool); err == nil {
			return execCommand{exec.Command(path, append([]string{exe}, args...)...)}, nil
		}
	}
	return nil, errors.New("this needs root, and neither sudo nor doas is installed; run it as root")
}

type execCommand struct{ *exec.Cmd }

func (c execCommand) SetStdin(r io.Reader)  { c.Stdin = r }
func (c execCommand) SetStdout(w io.Writer) { c.Stdout = w }
func (c execCommand) SetStderr(w io.Writer) { c.Stderr = w }
