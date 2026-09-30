package platform

import (
	"testing"
	"unsafe"
)

// Run itself isn't tested: it would pop a real UAC prompt.
func TestShellExecuteInfoMatchesWindows(t *testing.T) {
	want := uintptr(112) // sizeof(SHELLEXECUTEINFOW) on 64-bit Windows
	if unsafe.Sizeof(uintptr(0)) == 4 {
		want = 60
	}
	if got := unsafe.Sizeof(shellExecuteInfo{}); got != want {
		t.Errorf("shellExecuteInfo is %d bytes, SHELLEXECUTEINFOW is %d", got, want)
	}
	if got := unsafe.Offsetof(shellExecuteInfo{}.process); got != want-unsafe.Sizeof(uintptr(0)) {
		t.Errorf("hProcess is at %d, want the last field", got)
	}
}
