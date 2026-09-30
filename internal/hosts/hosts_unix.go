//go:build unix

package hosts

import "runtime"

// Path is the hosts file.
func Path() string { return "/etc/hosts" }

// FlushDNS clears the system's DNS cache, where it keeps one, so the block
// works straight away. It needs root.
func FlushDNS() {
	switch runtime.GOOS {
	case "darwin":
		flush("dscacheutil", "-flushcache")
		flush("killall", "-HUP", "mDNSResponder")
	case "linux":
		flush("resolvectl", "flush-caches") // systemd-resolved; elsewhere there's no cache to clear
	}
}
