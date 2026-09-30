package platform

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// IsAdmin reports whether mindful-yt runs elevated.
func IsAdmin() bool { return windows.GetCurrentProcessToken().IsElevated() }

// AdminCommand runs exe elevated, once the user allows it in Windows's UAC
// prompt, in a hidden window. Its Run waits for exe to finish.
func AdminCommand(exe string, args ...string) (Command, error) {
	return &elevated{exe, args}, nil
}

type elevated struct {
	exe  string
	args []string
}

// Windows shows its own prompt and exe gets a console of its own, so there's
// nothing to connect.
func (*elevated) SetStdin(io.Reader)  {}
func (*elevated) SetStdout(io.Writer) {}
func (*elevated) SetStderr(io.Writer) {}

// x/sys/windows has ShellExecute but not ShellExecuteEx, which is the one
// that returns a handle to wait on.
var procShellExecuteEx = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")

// shellExecuteInfo is SHELLEXECUTEINFOW.
type shellExecuteInfo struct {
	size          uint32
	mask          uint32
	hwnd          windows.Handle
	verb          *uint16
	file          *uint16
	parameters    *uint16
	directory     *uint16
	show          int32
	instApp       windows.Handle
	idList        uintptr
	class         *uint16
	keyClass      windows.Handle
	hotKey        uint32
	iconOrMonitor windows.Handle
	process       windows.Handle
}

const (
	seeMaskNoCloseProcess = 0x00000040 // fill in process
	seeMaskNoAsync        = 0x00000100 // don't return before the process has started
)

func (e *elevated) Run() error {
	quoted := make([]string, len(e.args))
	for i, arg := range e.args {
		quoted[i] = windows.EscapeArg(arg)
	}
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(e.exe)
	if err != nil {
		return err
	}
	params, err := windows.UTF16PtrFromString(strings.Join(quoted, " "))
	if err != nil {
		return err
	}
	info := shellExecuteInfo{
		mask: seeMaskNoCloseProcess | seeMaskNoAsync,
		verb: verb, file: file, parameters: params, show: windows.SW_HIDE,
	}
	info.size = uint32(unsafe.Sizeof(info))
	if ok, _, err := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info))); ok == 0 {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return ErrDeclined
		}
		return err
	}
	if info.process == 0 {
		return errors.New("windows didn't start " + e.exe)
	}
	defer windows.CloseHandle(info.process)
	if _, err := windows.WaitForSingleObject(info.process, windows.INFINITE); err != nil {
		return err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(info.process, &code); err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("exit status %d", code)
	}
	return nil
}
