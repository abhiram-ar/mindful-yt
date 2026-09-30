//go:build unix

package platform

import (
	"fmt"
	"os/exec"
	"runtime"
	"syscall"
	"time"
)

// Open opens a file or folder with the desktop's default app.
func Open(path string) error {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	cmd := exec.Command(opener, path)
	if err := cmd.Start(); err != nil {
		return err
	}
	// Openers normally hand the file to an app and exit straight away. Report
	// it if one fails, but don't hang on one that stays running.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("%s: %w", opener, err)
		}
	case <-time.After(3 * time.Second):
	}
	return nil
}

// RefreshPath does nothing here: package managers install into folders that
// are already on PATH.
func RefreshPath() {}

// NewProcessGroup starts cmd in a process group of its own, so KillTree can
// stop it together with its children.
func NewProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// KillTree stops a process started with NewProcessGroup, and its children.
// It asks first: yt-dlp's PyInstaller launcher only deletes the files it
// unpacked into the temp folder when it exits cleanly. Anything still running
// after three seconds is killed.
func KillTree(pid int) error {
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		return err
	}
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if syscall.Kill(-pid, 0) != nil { // the whole group is gone
			return nil
		}
	}
	return syscall.Kill(-pid, syscall.SIGKILL)
}
