//go:build unix

package platform

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestKillTreeStopsChildren(t *testing.T) {
	// A shell that starts a child, like yt-dlp starting ffmpeg, then waits.
	cmd := exec.Command("sh", "-c", "sleep 60 & echo $!; wait")
	NewProcessGroup(cmd)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 32)
	n, _ := out.Read(buf)
	child, err := strconv.Atoi(strings.TrimSpace(string(buf[:n])))
	if err != nil {
		t.Fatalf("child pid: %q", buf[:n])
	}

	// Reap the shell as it exits, the way exec.Cmd.Wait does while Cancel runs.
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	start := time.Now()
	if err := KillTree(cmd.Process.Pid); err != nil {
		t.Fatal(err)
	}
	<-waited
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("SIGTERM should have been enough, but stopping took %s", took)
	}
	for deadline := time.Now().Add(5 * time.Second); alive(child); time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("child %d is still running", child)
		}
	}
}

// alive reports whether pid is running. A killed child can linger as a zombie
// until init reaps it; that counts as stopped.
func alive(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return true // no /proc (macOS): trust kill
	}
	// The state letter follows the ")" that closes the command name.
	i := strings.LastIndexByte(string(stat), ')')
	return !(i >= 0 && i+2 < len(stat) && stat[i+2] == 'Z')
}
