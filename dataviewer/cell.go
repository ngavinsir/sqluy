package dataviewer

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/rivo/uniseg"
)

type (
	Cell struct {
		*tview.Box
		text       string
		textColor  tcell.Color
		bgColor    tcell.Color
		isFirstRow bool
	}
)

func NewCell(text string, isFirstRow bool, textColor, bgColor, borderColor tcell.Color) *Cell {
	return &Cell{
		Box:        tview.NewBox().SetBorder(true).SetBorderColor(borderColor).SetBackgroundColor(bgColor),
		text:       text,
		textColor:  textColor,
		bgColor:    bgColor,
		isFirstRow: isFirstRow,
	}
}

func (c *Cell) Draw(screen tcell.Screen) {
	c.Box.DrawForSubclass(screen, c)

	x, y, w, h := c.Box.GetInnerRect()

	textX := x
	textY := y
	if c.isFirstRow {
		textY++
	}
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
		screen.SetContent(textX, textY, runes[0], runes[1:], tcell.StyleDefault.Foreground(c.textColor).Background(c.bgColor))
		textX += textWidth
	}
}
