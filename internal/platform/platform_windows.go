// Package platform holds the Windows-specific pieces: opening files,
// re-reading PATH after an install, and stopping process trees.
package platform

import (
	"os"
	"os/exec"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Open opens a file or folder the way double-clicking it would.
func Open(path string) error {
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL)
}

// RefreshPath picks up the PATH entries an installer just added, without
// needing a new terminal.
func RefreshPath() {
	var parts []string
	for _, k := range []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`},
		{registry.CURRENT_USER, `Environment`},
	} {
		key, err := registry.OpenKey(k.root, k.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		value, _, err := key.GetStringValue("Path")
		key.Close()
		if err != nil {
			continue
		}
		if expanded, err := registry.ExpandString(value); err == nil {
			value = expanded
		}
		parts = append(parts, value)
	}
	parts = append(parts, os.Getenv("PATH"))
	os.Setenv("PATH", strings.Join(parts, string(os.PathListSeparator)))
}

// KillTree stops a process and everything it started.
func KillTree(pid int) error {
	return exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
}
