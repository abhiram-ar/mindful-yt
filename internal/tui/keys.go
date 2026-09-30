package tui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"
)

// The keys the screens respond to. Each screen's help line lists the ones it
// uses. Several share a key and differ in what the help line calls them.
var (
	keyInterrupt = key.NewBinding(key.WithKeys("ctrl+c"))
	keyUp        = key.NewBinding(key.WithKeys("up", "k"))
	keyDown      = key.NewBinding(key.WithKeys("down", "j"))
	keyChoose    = key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "choose")) // labels keyUp and keyDown

	keyContinue = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "continue"))
	keySelect   = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select"))
	keyInstall  = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "install"))
	keyStart    = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "start download"))
	keyPlay     = key.NewBinding(key.WithKeys("enter", "p"), key.WithHelp("enter", "play"))
	keyOpen     = key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open folder"))
	keyYes      = key.NewBinding(key.WithKeys("y"))
	keyNo       = key.NewBinding(key.WithKeys("n"))

	keyQuit       = key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("esc", "quit"))
	keyQuitTyping = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "quit")) // q is typing here
	keyQuitDone   = key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("q", "quit"))
	keyCancel     = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel"))
)

// helpLine lists what bindings do, on one faint line of at most width
// columns; what doesn't fit is left off.
func helpLine(width int, bindings ...key.Binding) string {
	h := help.New()
	h.ShortSeparator = " · "
	h.Styles.ShortKey, h.Styles.ShortDesc, h.Styles.ShortSeparator, h.Styles.Ellipsis = faint, faint, faint, faint
	h.SetWidth(width)
	// help drops the items that don't fit, but keeps one whose "…" wouldn't fit either.
	return ansi.Truncate(h.ShortHelpView(bindings), width, "…")
}
