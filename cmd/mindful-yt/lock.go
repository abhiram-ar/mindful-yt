package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"syscall"

	"github.com/charmbracelet/x/term"

	"github.com/abhiram-ar/mindful-yt/internal/hosts"
	"github.com/abhiram-ar/mindful-yt/internal/tui"
)

// writeHostsFlag makes lock-me-in write the block lines without asking. It's
// what mindful-yt runs as admin once the user has said yes twice, so it's
// left out of the usage.
const writeHostsFlag = "--write-hosts"

// lockMeIn blocks YouTube in the hosts file. It runs before anything looks at
// mindful-yt's own folders: as admin it may be another user, or root.
func lockMeIn(args []string) int {
	path := hosts.Path()
	switch {
	case len(args) == 1 && args[0] == writeHostsFlag:
		if _, err := hosts.Append(path); err != nil {
			fmt.Fprintf(os.Stderr, "Couldn't update %s: %v\n", path, err)
			return 1
		}
		hosts.FlushDNS()
		return 0
	case len(args) > 0:
		fmt.Fprintln(os.Stderr, "lock-me-in takes no options.")
		return 2
	}

	content, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "Couldn't read %s: %v\n", path, err)
		return 1
	}
	missing, err := hosts.Check(content)
	switch {
	case err != nil:
		fmt.Fprintf(os.Stderr, "Can't block YouTube: %v.\n", err)
		return 1
	case len(missing) == 0:
		fmt.Println("YouTube is already blocked on your machine. You're good to go.")
		return 0
	}
	if !term.IsTerminal(os.Stdin.Fd()) {
		fmt.Fprintln(os.Stderr, "mindful-yt lock-me-in needs an interactive terminal.")
		return 2
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGHUP)
	defer cancel()
	code, err := tui.RunLock(tui.Lock{
		Ctx: ctx, Path: path, Lines: missing, Helper: []string{exe, "lock-me-in", writeHostsFlag},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return code
}
