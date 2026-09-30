package platform

import (
	"errors"
	"io"
)

// Command runs something that may take over the terminal, like sudo asking
// for a password. It has the methods of Bubble Tea's ExecCommand, so tea.Exec
// can run it.
type Command interface {
	Run() error
	SetStdin(io.Reader)
	SetStdout(io.Writer)
	SetStderr(io.Writer)
}

// ErrDeclined means the user said no when the OS asked for permission.
var ErrDeclined = errors.New("permission declined")
