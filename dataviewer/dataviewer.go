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
		headers []string
		rows    []map[string]any
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
		Box:     tview.NewBox(),
		headers: headers[:3],
		rows:    items[:13],
	}
}

func (d *Dataviewer) Draw(screen tcell.Screen) {
	d.Box.DrawForSubclass(screen, d)

	x, y, _, _ := d.Box.GetInnerRect()
	textX := x
	textY := y
	for i, header := range d.headers {
		colWidth := d.getColWidth(header)
		c := NewCell(header, tcell.StyleDefault)
		c.SetRect(textX, textY, colWidth+2, 3)
		c.Draw(screen)

		// top left junction
		if i > 0 {
			screen.SetContent(textX, textY, tview.Borders.TopT, nil, tcell.StyleDefault)
		}

		textX += colWidth + 1
	}
	textY += 2
	textX = x

	for i, r := range d.rows {
		for j, header := range d.headers {
			v, ok := r[header]
			if !ok {
				continue
			}

			colWidth := d.getColWidth(header)
			c := NewCell(fmt.Sprintf("%+v", v), tcell.StyleDefault)
			c.SetRect(textX, textY, colWidth+2, 3)
			c.Draw(screen)

			// top left junction
			if j > 0 {
				screen.SetContent(textX, textY, tview.Borders.Cross, nil, tcell.StyleDefault)
			} else {
				screen.SetContent(textX, textY, tview.Borders.LeftT, nil, tcell.StyleDefault)
			}

			// bottom left juction
			if i >= len(d.rows)-1 && j > 0 {
				screen.SetContent(textX, textY+2, tview.Borders.BottomT, nil, tcell.StyleDefault)
			}

			// top right junction
			if j >= len(d.headers)-1 {
				screen.SetContent(textX+colWidth+1, textY, tview.Borders.RightT, nil, tcell.StyleDefault)
			}
			textX += colWidth + 1
		}
		textY += 2
		textX = x
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
