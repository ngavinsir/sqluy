package dataviewer

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/rivo/uniseg"
)

type (
	Cell struct {
		*tview.Box
		text      string
		textStyle tcell.Style
	}
)

func NewCell(text string, textStyle tcell.Style) *Cell {
	return &Cell{
		Box:       tview.NewBox().SetBorder(true),
		text:      text,
		textStyle: textStyle,
	}
}

func (c *Cell) Draw(screen tcell.Screen) {
	c.Box.DrawForSubclass(screen, c)

	x, y, w, h := c.Box.GetInnerRect()

	textX := x
	textY := y
	state := -1
	s := c.text
	boundaries := 0
	cluster := ""
	for s != "" {
		cluster, s, boundaries, state = uniseg.StepString(s, state)
		textWidth := boundaries >> uniseg.ShiftWidth
		if textX+textWidth > x+w {
			textY++
			textX = x
		}
		if textY >= y+h {
			break
		}

		runes := []rune(cluster)
		screen.SetContent(textX, textY, runes[0], runes[1:], c.textStyle)
		textX += textWidth
	}
}
