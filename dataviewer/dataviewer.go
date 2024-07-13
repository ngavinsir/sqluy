package dataviewer

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/rivo/uniseg"
)

type (
	Dataviewer struct {
		*tview.Box
		headers     []string
		rows        []map[string]any
		bgColor     tcell.Color
		borderColor tcell.Color
		textColor   tcell.Color

		cursor  [2]int
		offsets [2]int
	}
)

//go:embed sample.json
var sampleItems []byte

func New() *Dataviewer {
	var items []map[string]any
	err := json.Unmarshal(sampleItems, &items)
	if err != nil {
		panic(err)
	}

	var headers []string
	m := make(map[string]struct{})
	for _, i := range items {
		for k := range i {
			m[k] = struct{}{}
		}
	}
	for k := range m {
		headers = append(headers, k)
	}

	return &Dataviewer{
		Box:         tview.NewBox(),
		headers:     []string{"university", "birthDate", "firstName", "lastName"},
		rows:        items[:13],
		bgColor:     tview.Styles.PrimitiveBackgroundColor,
		borderColor: tcell.ColorGray,
		textColor:   tcell.ColorWhite,
	}
}

func (d *Dataviewer) Draw(screen tcell.Screen) {
	d.Box.DrawForSubclass(screen, d)

	x, y, _, _ := d.Box.GetInnerRect()
	textX := x
	textY := y
	textY += 2
	textX = x

	for i, r := range d.rows {
		firstRowOffset := 0
		if i == 0 {
			firstRowOffset = 1
		}

		for j, header := range d.headers {
			v, ok := r[header]
			if !ok {
				continue
			}

			colWidth := d.getColWidth(header)
			c := NewCell(fmt.Sprintf("%+v", v), i == 0, d.textColor, d.bgColor, d.borderColor)
			c.SetRect(textX, textY, colWidth+2, 3+firstRowOffset)
			c.Draw(screen)

			// top left junction
			if j > 0 {
				screen.SetContent(textX, textY, tview.Borders.Cross, nil, tcell.StyleDefault.Foreground(d.borderColor).Background(d.bgColor))
			} else {
				screen.SetContent(textX, textY, tview.Borders.LeftT, nil, tcell.StyleDefault.Foreground(d.borderColor).Background(d.bgColor))
			}

			// bottom left juction
			if i >= len(d.rows)-1 && j > 0 {
				screen.SetContent(textX, textY+2, tview.Borders.BottomT, nil, tcell.StyleDefault.Foreground(d.borderColor).Background(d.bgColor))
			}

			// top right junction
			if j >= len(d.headers)-1 {
				screen.SetContent(textX+colWidth+1, textY, tview.Borders.RightT, nil, tcell.StyleDefault.Foreground(d.borderColor).Background(d.bgColor))
			}
			textX += colWidth + 1
		}
		textY += 2 + firstRowOffset
		textX = x
	}

	// header
	textY = y
	for i, header := range d.headers {
		colWidth := d.getColWidth(header)
		c := NewCell(header, false, d.bgColor, d.textColor, d.borderColor)
		c.SetRect(textX, textY, colWidth+2, 3)
		c.Draw(screen)

		// top left and bottom left junction
		if i > 0 {
			screen.SetContent(textX, textY, tview.Borders.TopT, nil, tcell.StyleDefault.Foreground(d.borderColor).Background(d.textColor))
			screen.SetContent(textX, textY+2, tview.Borders.BottomT, nil, tcell.StyleDefault.Foreground(d.borderColor).Background(d.textColor))
		}

		textX += colWidth + 1
	}
}

func (d *Dataviewer) getColWidth(header string) int {
	maxWidth := uniseg.StringWidth(header)
	for _, r := range d.rows {
		v, ok := r[header]
		if !ok {
			continue
		}
		width := uniseg.StringWidth(fmt.Sprintf("%+v", v))
		if width > maxWidth {
			maxWidth = width
		}
	}
	return maxWidth
}
