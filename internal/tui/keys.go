package tui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Quit         key.Binding
	Tab          key.Binding
	Up           key.Binding
	Down         key.Binding
	HalfPageUp   key.Binding
	HalfPageDown key.Binding
	Top          key.Binding
	Bottom       key.Binding
	NextComment  key.Binding
	PrevComment  key.Binding
	Cancel       key.Binding
	Confirm      key.Binding
	VisualMode   key.Binding
}

var keys = keyMap{
	Quit:         key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	Tab:          key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch pane")),
	Up:           key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("k", "up")),
	Down:         key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("j", "down")),
	HalfPageUp:   key.NewBinding(key.WithKeys("ctrl+u", "shift+up"), key.WithHelp("shift+up", "half page up")),
	HalfPageDown: key.NewBinding(key.WithKeys("ctrl+d", "shift+down"), key.WithHelp("shift+down", "half page down")),
	Top:          key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "top")),
	Bottom:       key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
	NextComment:  key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next comment")),
	PrevComment:  key.NewBinding(key.WithKeys("["), key.WithHelp("[", "prev comment")),
	Cancel:       key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	Confirm:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm")),
	VisualMode:   key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "select lines")),
}
