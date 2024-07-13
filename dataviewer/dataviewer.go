package dataviewer

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"time"

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

func New(app *tview.Application) *Dataviewer {
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

	d := &Dataviewer{
		Box:         tview.NewBox().SetBorder(true).SetTitle("Dataviewer").SetTitleAlign(tview.AlignLeft),
		headers:     []string{"university", "birthDate", "firstName", "lastName"},
		rows:        items[:13],
		bgColor:     tview.Styles.PrimitiveBackgroundColor,
		borderColor: tcell.ColorGray,
		textColor:   tcell.ColorWhite,
		cursor:      [2]int{3, 0},
		offsets:     [2]int{0, 0},
	}

	go func() {
		for {
			time.Sleep(500 * time.Millisecond)
			app.QueueUpdateDraw(func() {
				d.cursor[1]++
				if d.cursor[1] > len(d.headers)-1 {
					d.cursor[0]++
					d.cursor[1] = 0
				}
				if d.cursor[0] > len(d.rows) {
					d.cursor[0] = 0
				}
			})
		}
	}()

	return d
}

func (d *Dataviewer) Draw(screen tcell.Screen) {
	d.Box.DrawForSubclass(screen, d)

	x, y, w, h := d.Box.GetInnerRect()
	textX := x
	textY := y
	textY += 2
	textX = x
	defer func() {
		tview.Print(screen, fmt.Sprintf("%+v", d.offsets), x, y+h, 10, tview.AlignLeft, tcell.ColorWhite)
		tview.Print(screen, fmt.Sprintf("%+v", d.cursor), x, y+h+1, 10, tview.AlignLeft, tcell.ColorWhite)
	}()

	// adjust offset if cursor hidden on the left
	if d.cursor[1] < d.offsets[1] {
		d.offsets[1] = d.cursor[1]
	}

	// adjust offset if cursor hidden on the right
	width := x
rightOffset:
	for d.offsets[1] < d.cursor[1] {
		for i, h := range d.headers[d.offsets[1] : d.cursor[1]+1] {
			colWidth := d.getColTextWidth(h)
			if width+colWidth+1 > x+w+1 {
				d.offsets[1]++
				width = x
				break
			}
			if i >= d.cursor[1]-d.offsets[1] {
				break rightOffset
			}
			width += colWidth + 1
		}
	}

	// adjust offset if cursor hidden on the top
	if d.cursor[0] < d.offsets[0] {
		d.offsets[0] = d.cursor[0]
	}

	// adjust offset if cursor hidden on the bottom
	height := y + d.getHeaderHeight() + 2
	// return early if box height is too short
	if height >= y+h {
		return
	}
bottomOffset:
	for d.offsets[0] < d.cursor[0] {
		for i, r := range d.rows[d.offsets[0] : d.cursor[0]+1] {
			// measure max text height on the row
			textHeight := 1
			for _, header := range d.headers {
				v, ok := r[header]
				if !ok {
					continue
				}
				text := fmt.Sprintf("%+v", v)
				th := d.getTextHeight(text, w-2)
				if th > textHeight {
					textHeight = th
				}
			}

			if height+textHeight+1 > y+h+1 {
				d.offsets[0]++
				height = y + d.getHeaderHeight() + 2
				break
			}
			if i >= d.cursor[0]-d.offsets[0] {
				break bottomOffset
			}
			height += textHeight + 1
		}
	}

	for i, r := range d.rows[d.offsets[0]:] {
		i += d.offsets[0]
		firstRowOffset := 0
		if i == d.offsets[0] {
			firstRowOffset = 1
		}

		if textY+2+firstRowOffset >= y+h {
			break
		}

		// measure max text height on the row
		textHeight := 1
		for _, header := range d.headers {
			v, ok := r[header]
			if !ok {
				continue
			}
			text := fmt.Sprintf("%+v", v)
			th := d.getTextHeight(text, w-2)
			if th > textHeight {
				textHeight = th
			}
		}

		for j, header := range d.headers[d.offsets[1]:] {
			j += d.offsets[1]
			if textX >= x+w-1 {
				break
			}

			v, ok := r[header]
			if !ok {
				continue
			}
			text := fmt.Sprintf("%+v", v)

			colWidth := d.getColWidth(header)

			if d.cursor == [2]int{i + 1, j} {
				defer d.drawCell(screen, i, j, textX, textY, colWidth, 2+textHeight, firstRowOffset, text)
			} else {
				d.drawCell(screen, i, j, textX, textY, colWidth, 2+textHeight, firstRowOffset, text)
			}
			textX += colWidth + 1
		}
		textY += 1 + textHeight + firstRowOffset
		textX = x
	}

	// header
	textY = y
	headerHeight := d.getHeaderHeight()

	for i, header := range d.headers {
		if i < d.offsets[1] {
			continue
		}
		if textX >= x+w-1 {
			break
		}

		colWidth := d.getColWidth(header)

		if d.cursor == [2]int{0, i} {
			defer d.drawHeader(screen, i, textX, textY, colWidth, 2+headerHeight, header)
		} else {
			d.drawHeader(screen, i, textX, textY, colWidth, 2+headerHeight, header)
		}

		textX += colWidth + 1
	}
}

func (d *Dataviewer) getColTextWidth(header string) int {
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

func (d *Dataviewer) getTextHeight(text string, w int) int {
	textX := 0
	textY := 0

	state := -1
	s := text
	boundaries := 0
	for s != "" {
		_, s, boundaries, state = uniseg.StepString(s, state)
		textWidth := boundaries >> uniseg.ShiftWidth
		if textX+textWidth > w {
			textY++
			textX = 0
			continue
		}
		textX += textWidth
	}
	return textY + 1
}

func (d *Dataviewer) getColWidth(header string) int {
	i := slices.Index(d.headers, header)
	x, _, w, _ := d.Box.GetInnerRect()

	emptyHorizontalSpace := 0
	width := x
	textX := x
	for j, header := range d.headers[d.offsets[1]:] {
		j += d.offsets[1]
		width += d.getColTextWidth(header) + 1
		if j < i {
			textX += d.getColTextWidth(header) + 1
		}
	}
	if width < w+x {
		emptyHorizontalSpace = w + x - width - 1
	}

	colWidth := d.getColTextWidth(header)
	isLastCol := i == len(d.headers)-1

	if emptyHorizontalSpace > 0 && !isLastCol {
		colWidth += emptyHorizontalSpace / (len(d.headers) - d.offsets[1])
	} else if emptyHorizontalSpace > 0 {
		colWidth += emptyHorizontalSpace - (emptyHorizontalSpace/(len(d.headers)-d.offsets[1]))*(len(d.headers)-d.offsets[1]-1)
	}

	// if the next header width is too wide, extend the current header width until the edge
	// or if it's the first/last header and it's too wide
	if ((i == 0 || isLastCol) && x+colWidth+1 >= x+w) || (!isLastCol && textX+colWidth+1+d.getColTextWidth(d.headers[i+1])+1 >= x+w) {
		colWidth = w - textX - 1
	}

	return colWidth
}

func (d *Dataviewer) getHeaderHeight() int {
	_, _, w, _ := d.Box.GetInnerRect()
	textHeight := 1
	for _, header := range d.headers {
		th := d.getTextHeight(header, w-2)
		if th > textHeight {
			textHeight = th
		}
	}
	return textHeight
}

func (d *Dataviewer) drawCell(screen tcell.Screen, i, j, x, y, colWidth, height, topPadding int, content string) {
	textColor := d.textColor
	borderColor := d.borderColor
	bgColor := d.bgColor
	if d.cursor == [2]int{i + 1, j} {
		textColor = tcell.ColorBlack
		borderColor = tcell.ColorBlack
		bgColor = tcell.ColorYellow
	}
	c := NewCell(content, x, y, colWidth+2, height, topPadding, textColor, bgColor, borderColor)
	c.Draw(screen)

	// top left junction
	if j > 0 {
		screen.SetContent(x, y, tview.Borders.Cross, nil, tcell.StyleDefault.Foreground(borderColor).Background(bgColor))
	} else {
		screen.SetContent(x, y, tview.Borders.LeftT, nil, tcell.StyleDefault.Foreground(borderColor).Background(bgColor))
	}

	// top right junction
	if j >= len(d.headers)-1 {
		screen.SetContent(x+colWidth+1, y, tview.Borders.RightT, nil, tcell.StyleDefault.Foreground(borderColor).Background(bgColor))
	} else {
		screen.SetContent(x+colWidth+1, y, tview.Borders.Cross, nil, tcell.StyleDefault.Foreground(borderColor).Background(bgColor))
	}

	// bottom left juction
	if i >= len(d.rows)-1 && j > 0 {
		screen.SetContent(x, y-1+height+topPadding, tview.Borders.BottomT, nil, tcell.StyleDefault.Foreground(borderColor).Background(bgColor))
	} else if j > 0 {
		screen.SetContent(x, y-1+height+topPadding, tview.Borders.Cross, nil, tcell.StyleDefault.Foreground(borderColor).Background(bgColor))
	} else if i >= len(d.rows)-1 {
		screen.SetContent(x, y-1+height+topPadding, tview.Borders.BottomLeft, nil, tcell.StyleDefault.Foreground(borderColor).Background(bgColor))
	} else {
		screen.SetContent(x, y-1+height+topPadding, tview.Borders.LeftT, nil, tcell.StyleDefault.Foreground(borderColor).Background(bgColor))
	}

	// top right junction
	if j >= len(d.headers)-1 {
		screen.SetContent(x+colWidth+1, y, tview.Borders.RightT, nil, tcell.StyleDefault.Foreground(borderColor).Background(bgColor))
	} else {
		screen.SetContent(x+colWidth+1, y, tview.Borders.Cross, nil, tcell.StyleDefault.Foreground(borderColor).Background(bgColor))
	}

	// bottom right junction
	if i >= len(d.rows)-1 && j < len(d.headers)-1 {
		screen.SetContent(x+colWidth+1, y-1+height+topPadding, tview.Borders.BottomT, nil, tcell.StyleDefault.Foreground(borderColor).Background(bgColor))
	} else if j < len(d.headers)-1 {
		screen.SetContent(x+colWidth+1, y-1+height+topPadding, tview.Borders.Cross, nil, tcell.StyleDefault.Foreground(borderColor).Background(bgColor))
	} else if i >= len(d.rows)-1 {
		screen.SetContent(x+colWidth+1, y-1+height+topPadding, tview.Borders.BottomRight, nil, tcell.StyleDefault.Foreground(borderColor).Background(bgColor))
	} else {
		screen.SetContent(x+colWidth+1, y-1+height+topPadding, tview.Borders.RightT, nil, tcell.StyleDefault.Foreground(borderColor).Background(bgColor))
	}
}

func (d *Dataviewer) drawHeader(screen tcell.Screen, i, x, y, colWidth, height int, header string) {
	textColor := d.bgColor
	borderColor := d.borderColor
	bgColor := d.textColor
	if d.cursor == [2]int{0, i} {
		textColor = tcell.ColorBlack
		borderColor = tcell.ColorBlack
		bgColor = tcell.ColorYellow
	}
	c := NewCell(header, x, y, colWidth+2, height, 0, textColor, bgColor, borderColor)
	c.Draw(screen)

	// top left junction
	if i > 0 {
		screen.SetContent(x, y, tview.Borders.TopT, nil, tcell.StyleDefault.Foreground(borderColor).Background(bgColor))
	} else {
		screen.SetContent(x, y, tview.Borders.TopLeft, nil, tcell.StyleDefault.Foreground(borderColor).Background(bgColor))
	}

	// top right junction
	if i >= len(d.headers)-1 {
		screen.SetContent(x+colWidth+1, y, tview.Borders.TopRight, nil, tcell.StyleDefault.Foreground(borderColor).Background(bgColor))
	} else {
		screen.SetContent(x+colWidth+1, y, tview.Borders.TopT, nil, tcell.StyleDefault.Foreground(borderColor).Background(bgColor))
	}

	// bottom left junction
	if i > 0 {
		screen.SetContent(x, y-1+height, tview.Borders.BottomT, nil, tcell.StyleDefault.Foreground(borderColor).Background(bgColor))
	} else {
		screen.SetContent(x, y-1+height, tview.Borders.BottomLeft, nil, tcell.StyleDefault.Foreground(borderColor).Background(bgColor))
	}

	// bottom right junction
	if i >= len(d.headers)-1 {
		screen.SetContent(x+colWidth+1, y-1+height, tview.Borders.BottomRight, nil, tcell.StyleDefault.Foreground(borderColor).Background(bgColor))
	} else {
		screen.SetContent(x+colWidth+1, y-1+height, tview.Borders.BottomT, nil, tcell.StyleDefault.Foreground(borderColor).Background(bgColor))
	}
}
