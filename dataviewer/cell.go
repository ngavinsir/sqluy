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
		topPadding int
	}
)

var cellPool = sync.Pool{
    New: func() interface{} {
        return &Cell{
            Box: tview.NewBox(),
        }
    },
}

func NewCell(text string, x, y, w, h, topPadding int, textColor, bgColor, borderColor tcell.Color) *Cell {
    cell := cellPool.Get().(*Cell)
    cell.Box.SetBorder(true).
        SetBorderColor(borderColor).
        SetBackgroundColor(bgColor).
        SetRect(x, y, w, h+topPadding)
    cell.text = text
    cell.textColor = textColor
    cell.bgColor = bgColor
    cell.topPadding = topPadding
    return cell
}

func (c *Cell) Release() {
    cellPool.Put(c)
}

func (c *Cell) Draw(screen tcell.Screen) {
	c.Box.DrawForSubclass(screen, c)

	x, y, w, h := c.Box.GetInnerRect()

	textX := x
	textY := y
	if c.topPadding > 0 {
		textY += c.topPadding
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
