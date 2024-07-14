package dataviewer

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/rivo/uniseg"
)

type (
	Dataviewer struct {
		*tview.Box
		colWidths     []int
		headers       []string
		rows          []map[string]any
		rowHeights    []int
		offsets       [2]int
		cursor        [2]int
		visibleLeft   int
		visibleTop    int
		visibleBottom int
		visibleRight  int
		textColor     tcell.Color
		borderColor   tcell.Color
		bgColor       tcell.Color
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
		Box: tview.NewBox().SetBorder(true).SetTitle("Dataviewer").SetTitleAlign(tview.AlignLeft),
		// headers: headers,
		headers:      []string{"username", "bank", "crypto", "macAddress", "weight", "email", "role", "age", "gender", "ein", "height", "phone", "birthDate", "eyeColor", "password", "ip", "image", "id", "address", "lastName", "university", "bloodGroup", "firstName", "ssn", "company", "userAgent", "maidenName", "hair"},
		rows:         items[:30],
		bgColor:      tview.Styles.PrimitiveBackgroundColor,
		borderColor:  tcell.ColorGray,
		textColor:    tcell.ColorWhite,
		visibleLeft:  -1,
		visibleRight: -1,
	}
	fmt.Printf("headers: []string{\"%s\"}\n", strings.Join(headers, "\", \""))

	return d
}

func (d *Dataviewer) Draw(screen tcell.Screen) {
	defer func() {
		fmt.Printf("cursor: %+v, offsets: %+v\n", d.cursor, d.offsets)
		// fmt.Printf("vis left: %d, vis right: %d, colWidths: %+v\n", d.visibleLeft, d.visibleRight, d.colWidths)
	}()
	d.Box.DrawForSubclass(screen, d)
	fmt.Println("draw")

	x, y, w, h := d.Box.GetInnerRect()
	textX := x
	textY := y
	textY += 2
	textX = x
	defer func() {
		tview.Print(screen, fmt.Sprintf("%+v", d.offsets), x, y+h, 10, tview.AlignLeft, tcell.ColorWhite)
		tview.Print(screen, fmt.Sprintf("%+v", d.cursor), x, y+h+1, 10, tview.AlignLeft, tcell.ColorWhite)
	}()

	// adjust offset if cursor hidden on the top
	if d.cursor[0] < d.offsets[0] {
		d.offsets[0] = d.cursor[0]
	}

	// adjust offset if cursor is hidden on the left
	if d.getColWidth(d.cursor[1]) == 0 && d.cursor[1] < d.offsets[1] {
		d.offsets[1] = d.cursor[1]
		for d.offsets[1] > 0 {
			d.offsets[1]--
			d.visibleLeft = -1
			d.visibleRight = -1
			if b := d.getColWidth(d.cursor[1]); b == 0 {
				break
			}
		}
	}

	// adjust offset if cursor is hidden on the right
	for d.getColWidth(d.cursor[1]) == 0 && d.cursor[1] > d.offsets[1] {
		d.offsets[1]++
		d.visibleLeft = -1
		d.visibleRight = -1
	}

	// adjust offset if cursor hidden on the bottom
	height := y + d.getHeaderHeight() + 1
	// return early if box height is too short
	if height >= y+h {
		return
	}
bottomOffset:
	for d.offsets[0] < d.cursor[0] {
		for i, r := range d.rows[d.offsets[0] : d.cursor[0]+1] {
			i += d.offsets[0]
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

			fmt.Printf("bo i: %d, offsets: %+v, cursor: %+v, th: %d, height: %d, y: %d, hh: %d, h: %d\n", i, d.offsets, d.cursor, textHeight, height, y, d.getHeaderHeight(), h)

			// increment row offset if current row span until below bottom offset
			if height+textHeight+1 > y+h+1 {
				d.offsets[0]++
				height = y + d.getHeaderHeight() + 1
				break
			}

			// cursor is no longer hidden below, can break
			if i >= d.cursor[0] {
				break bottomOffset
			}

			height += textHeight + 1
		}
	}

	// draw rows
	for i, r := range d.rows[d.offsets[0]:] {
		i += d.offsets[0]
		firstRowOffset := 0
		if i == d.offsets[0] {
			firstRowOffset = 1
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

		if textY+1+textHeight+firstRowOffset >= y+h {
			break
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

			colWidth := d.getColWidth(j)
			if colWidth == 0 {
				break
			}

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

		colWidth := d.getColWidth(i)

		if d.cursor == [2]int{0, i} {
			defer d.drawHeader(screen, i, textX, textY, colWidth, 2+headerHeight, header)
		} else {
			d.drawHeader(screen, i, textX, textY, colWidth, 2+headerHeight, header)
		}

		textX += colWidth + 1
	}
}

func (d *Dataviewer) getColTextWidth(colIndex int) int {
	header := d.headers[colIndex]
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

func (d *Dataviewer) getColWidth(colIndex int) int {
	isColVisible := colIndex >= d.visibleLeft && colIndex <= d.visibleRight
	isCursorVisible := d.cursor[1] >= d.visibleLeft && d.cursor[1] <= d.visibleRight
	// if col and cursor is visible, returned cached width
	if isColVisible && isCursorVisible {
		return d.colWidths[colIndex-d.visibleLeft]
	}
	// if cursor is visible but col is not, then just return 0
	if isCursorVisible {
		return 0
	}

	// current cursor is hidden on the left, try to shift left as far as possible while making sure that col index is still visible
	// if d.cursor[1] < d.offsets[1] {
	// 	d.offsets[1] = d.cursor[1]
	// 	return d.getColWidth(colIndex, true)
	// }

	startIndex := d.offsets[1]
	lastIndex := d.offsets[1]
	x, _, w, _ := d.Box.GetInnerRect()
	width := x

	emptyHorizontalSpace := 0
	for j := range d.headers[d.offsets[1]:] {
		j += d.offsets[1]
		lastIndex = j

		// if the first width is already too wide, break
		if width+d.getColTextWidth(j)+1 >= x+w {
			break
		}

		width += d.getColTextWidth(j) + 1

		// stop if the next header is too wide
		if j < len(d.headers)-1 && width+d.getColTextWidth(j+1)+1 >= x+w {
			fmt.Println("next header is too wide")
			break
		}
	}
	emptyHorizontalSpace = w + x - width - 1

	// current cursor is hidden on the right and isHiddenLeft is true, use the latest one
	// if d.cursor[1] > lastIndex && isHiddenLeft {
	// 	d.offsets[1]++
	// 	return d.getColWidth(colIndex, false)
	// }

	d.visibleLeft = startIndex
	d.visibleRight = lastIndex

	if startIndex == lastIndex && width+d.getColTextWidth(startIndex)+1 >= x+w {
		d.colWidths = []int{emptyHorizontalSpace - 1}
	} else {
		d.colWidths = make([]int, lastIndex-startIndex+1)
		for a := range len(d.colWidths) {
			colWidth := d.getColTextWidth(a + startIndex)
			if emptyHorizontalSpace > 0 && a < len(d.colWidths)-1 {
				d.colWidths[a] = colWidth + emptyHorizontalSpace/(lastIndex-startIndex+1)
			} else if emptyHorizontalSpace > 0 {
				d.colWidths[a] = colWidth + emptyHorizontalSpace - (emptyHorizontalSpace/(lastIndex-startIndex+1))*(lastIndex-startIndex+1-1)
			} else {
				d.colWidths[a] = colWidth
			}
		}
	}

	// if isHiddenLeft try to shift left again until col index is no longer visible, then use the latest one
	// if isHiddenLeft && d.offsets[1] > 0 {
	// 	d.offsets[1]--
	// 	return d.getColWidth(colIndex, true)
	// }

	fmt.Printf("col index: %d,  colWidths: %+v, startIndex: %d, lastIndex: %d, x: %d, w: %d, width: %d, emptyHorizontalSpace: %d\n", colIndex, d.colWidths, startIndex, lastIndex, x, w, width, emptyHorizontalSpace)
	fmt.Printf("vis left: %d, vis right: %d, colWidths: %+v\n", d.visibleLeft, d.visibleRight, d.colWidths)

	if colIndex >= startIndex && colIndex <= lastIndex {
		return d.colWidths[colIndex-startIndex]
	}
	return 0
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

func (d *Dataviewer) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return d.Box.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		switch event.Key() {
		case tcell.KeyUp:
			d.cursor[0]--
			if d.cursor[0] < 0 {
				d.cursor[0] = 0
			}
		case tcell.KeyDown:
			d.cursor[0]++
			if d.cursor[0] > len(d.rows) {
				d.cursor[0] = len(d.rows)
			}
		case tcell.KeyLeft:
			d.cursor[1]--
			if d.cursor[1] < 0 {
				d.cursor[1] = 0
			}
		case tcell.KeyRight:
			d.cursor[1]++
			if d.cursor[1] > len(d.headers)-1 {
				d.cursor[1] = len(d.headers) - 1
			}
		}
	})
}
