package dataviewer

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type (
	Dataviewer struct {
		*tview.Box
		headers []string
		rows    []map[string]any
	}
)

func (d *Dataviewer) Draw(screen tcell.Screen) {
	d.Box.DrawForSubclass(screen, d)

}
