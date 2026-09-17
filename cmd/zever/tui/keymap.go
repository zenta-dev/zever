package tui

// Binding is one action in the shared keymap: the key strokes that trigger
// it plus the one-line help text. Keep Keys in msg.String() form
// ("enter", "esc", "ctrl+c", "?", "/", "j", "up", "space", ...).
type Binding struct {
	// Keys lists the accepted strokes. First entry is the canonical one
	// shown in help.
	Keys []string
	// Desc is the short help description, e.g. "select".
	Desc string
}

// Matches reports whether key (a msg.String() value) triggers b.
func (b Binding) Matches(key string) bool {
	for _, k := range b.Keys {
		if k == key {
			return true
		}
	}
	return false
}

// Keymap is the shared key contract for the zever TUI. The SAME value
// drives dispatch (Update) and help text (help.go), so help can never
// drift from behavior. Screen agents: reuse DefaultKeymap, do not fork it.
type Keymap struct {
	Up     Binding
	Down   Binding
	Select Binding
	Back   Binding
	Quit   Binding
	Help   Binding
	Filter Binding
}

// DefaultKeymap returns the consistent bindings used everywhere:
//
//	enter select, esc back, ctrl+c quit, ? help, / filter, j/k+arrows move.
func DefaultKeymap() Keymap {
	return Keymap{
		Up:     Binding{Keys: []string{"k", "up"}, Desc: "move up"},
		Down:   Binding{Keys: []string{"j", "down"}, Desc: "move down"},
		Select: Binding{Keys: []string{"enter"}, Desc: "select"},
		Back:   Binding{Keys: []string{"esc"}, Desc: "back"},
		Quit:   Binding{Keys: []string{"ctrl+c"}, Desc: "quit"},
		Help:   Binding{Keys: []string{"?"}, Desc: "help"},
		Filter: Binding{Keys: []string{"/"}, Desc: "filter"},
	}
}

// Rows returns the bindings in stable display order for help rendering.
func (k Keymap) Rows() []Binding {
	return []Binding{k.Up, k.Down, k.Select, k.Back, k.Quit, k.Help, k.Filter}
}
