package dataviewer

import (
	_ "embed"
	"fmt"
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/ngavinsir/sqluy/editor"
	"github.com/ngavinsir/sqluy/vim"
	"github.com/rivo/tview"
	"github.com/rivo/uniseg"
)

type (
	keymapper interface {
		Get(keys []string, group string) ([]string, bool)
	}

	Dataviewer struct {
		keymapper  keymapper
		runeRunner map[Action]func(r rune)
		*tview.Box
		operatorRunner   map[Action]func(target [2]int)
		motionRunner     map[Action]func() [2]int
		actionRunner     map[Action]func()
		searchEditor     *editor.Editor
		pending          []string
		rowHeights       []int
		rows             []map[string]string
		headers          []string
		colWidths        []int
		visualStart      [2]int
		offsets          [2]int
		cursor           [2]int
		lastMotion       Action
		borderColor      tcell.Color
		bgColor          tcell.Color
		pendingAction    Action
		textColor        tcell.Color
		pendingCount     int
		visibleRight     int
		visibleBottom    int
		visibleLeft      int
		visibleTop       int
		waitingForMotion bool
		mode             mode
	}
)

func New(km keymapper) *Dataviewer {
	d := &Dataviewer{
		keymapper:    km,
		Box:          tview.NewBox().SetBorder(true).SetTitle("Dataviewer").SetTitleAlign(tview.AlignLeft),
		bgColor:      tview.Styles.PrimitiveBackgroundColor,
		borderColor:  tcell.ColorGray,
		textColor:    tcell.ColorWhite,
		visibleLeft:  -1,
		visibleRight: -1,
	}

	d.operatorRunner = map[Action]func(target [2]int){
		ActionNone: d.MoveCursorTo,
	}

	d.motionRunner = map[Action]func() [2]int{
		ActionMoveEndOfLine:   d.GetEndOfLineCursor,
		ActionMoveStartOfLine: d.GetStartOfLineCursor,
		// ActionMoveFirstNonWhitespace: d.GetFirstNonWhitespaceCursor,
		ActionMoveDown:      d.GetDownCursor,
		ActionMoveUp:        d.GetUpCursor,
		ActionMoveLeft:      d.GetLeftCursor,
		ActionMoveRight:     d.GetRightCursor,
		ActionMoveLastLine:  d.GetLastLineCursor,
		ActionMoveFirstLine: d.GetFirstLineCursor,
		// ActionMoveStartOfWord:        d.GetStartOfWordCursor,
		// ActionMoveStartOfBigWord:     d.GetStartOfBigWordCursor,
		// ActionMoveEndOfBigWord:       d.GetEndOfBigWordCursor,
		// ActionMoveBackEndOfBigWord:   d.GetBackEndOfBigWordCursor,
		// ActionMoveBackStartOfBigWord: d.GetBackStartOfBigWordCursor,
		// ActionMoveEndOfWord:          d.GetEndOfWordCursor,
		// ActionMoveBackEndOfWord:      d.GetBackEndOfWordCursor,
		// ActionMoveBackStartOfWord:    d.GetBackStartOfWordCursor,
		// ActionEnableSearch:           d.EnableSearch,
		// ActionFlash:                  d.Flash,
		// ActionTil:                    d.GetTilCursor,
		// ActionTilBack:                d.GetTilBackCursor,
		// ActionFind:                   d.GetFindCursor,
		// ActionFindBack:               d.GetFindBackCursor,
		// ActionInside:                 d.GetInsideOrAroundCursor,
		// ActionAround:                 d.GetInsideOrAroundCursor,
	}

	return d
}

func (d *Dataviewer) SetData(headers []string, rows []map[string]string) {
	d.headers = headers
	d.rows = rows
	d.cursor = [2]int{0, 0}
	d.offsets = [2]int{0, 0}
	d.visibleLeft = -1
	d.visibleRight = -1
	clear(d.colWidths)
}

func (d *Dataviewer) Draw(screen tcell.Screen) {
	defer func() {
		fmt.Printf("cursor: %+v, offsets: %+v\n", d.cursor, d.offsets)
		// fmt.Printf("vis left: %d, vis right: %d, colWidths: %+v\n", d.visibleLeft, d.visibleRight, d.colWidths)
	}()
	d.Box.DrawForSubclass(screen, d)

	if d.headers == nil {
		return
	}

	x, y, w, h := d.Box.GetInnerRect()
	textX := x
	textY := y
	textY += d.getHeaderHeight() + 1
	textX = x
	defer func() {
		tview.Print(screen, fmt.Sprintf(" x:%d/%d y:%d/%d ", d.cursor[1], len(d.headers)-1, d.cursor[0], len(d.rows)), x+2, y+h, 20, tview.AlignLeft, tcell.ColorWhite)
	}()

	// adjust offset if cursor hidden on the top
	if d.cursor[0] < d.offsets[0]+1 {
		d.offsets[0] = d.cursor[0] - 1
		if d.offsets[0] < 0 {
			d.offsets[0] = 0
		}
	}

	// adjust offset if cursor is hidden on the left
	fmt.Println("adjust left")
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
	fmt.Println("adjust right")
	for d.getColWidth(d.cursor[1]) == 0 && d.cursor[1] > d.offsets[1] {
		d.offsets[1]++
		d.visibleLeft = -1
		d.visibleRight = -1
	}

	// adjust offset if cursor hidden on the bottom
	height := y + d.getHeaderHeight() + 2
	// return early if box height is too short
	if height >= y+h {
		return
	}
bottomOffset:
	for d.offsets[0] < d.cursor[0] {
		for i, r := range d.rows[d.offsets[0]:d.cursor[0]] {
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

			// increment row offset if current row span until below bottom offset
			if height+textHeight+1 >= y+h {
				d.offsets[0]++
				height = y + d.getHeaderHeight() + 2
				break
			}

			// cursor is no longer hidden below, can break
			if i >= d.cursor[0]-1 {
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

			fmt.Printf("draw row: %d, col: %d\n", i, j)
			colWidth := d.getColWidth(j)
			if colWidth == 0 {
				break
			}

			if d.HasFocus() && d.cursor == [2]int{i + 1, j} {
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

		if d.HasFocus() && d.cursor == [2]int{0, i} {
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

	d.visibleLeft = startIndex
	d.visibleRight = lastIndex

	if startIndex == lastIndex && width == x && width+d.getColTextWidth(startIndex)+1 >= x+w {
		d.colWidths = []int{emptyHorizontalSpace - 1}
	} else {
		d.colWidths = make([]int, lastIndex-startIndex+1)
		for a := range len(d.colWidths) {
			colWidth := d.getColTextWidth(a + startIndex)
			if emptyHorizontalSpace > 0 && a < len(d.colWidths)-1 {
				fmt.Println("$a")
				d.colWidths[a] = colWidth + emptyHorizontalSpace/(lastIndex-startIndex+1)
			} else if emptyHorizontalSpace > 0 {
				fmt.Println("$b")
				d.colWidths[a] = colWidth + emptyHorizontalSpace - (emptyHorizontalSpace/(lastIndex-startIndex+1))*(lastIndex-startIndex)
			} else {
				fmt.Println("$c")
				d.colWidths[a] = colWidth
			}
		}
	}

	// fmt.Printf("colIdx: %d, startIdx: %d, lastIdx: %d, x: %d, w: %d, width: %d, empty: %d\n", colIndex, startIndex, lastIndex, x, w, width, emptyHorizontalSpace)
	// fmt.Printf("vis start: %d, vis end: %d, colwidth: %d, col widths: %+v\n", d.visibleLeft, d.visibleRight, d.getColTextWidth(startIndex), d.colWidths)

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
	if d.HasFocus() && d.cursor == [2]int{i + 1, j} {
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
	if d.HasFocus() && d.cursor == [2]int{0, i} {
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
		eventName := event.Name()
		if event.Key() == tcell.KeyRune {
			eventName = string(event.Rune())
		} else {
			eventName = strings.ToLower(eventName)
		}
		d.pending = append(d.pending, eventName)

		isDigit := event.Key() == tcell.KeyRune && unicode.IsDigit(event.Rune())

		group := "r"
		if d.cursor[0] == 0 {
			group = "h"
		}

		actionStrings, anyStartWith := d.keymapper.Get(d.pending, group)
		if actionStrings == nil {
			actionStrings = []string{""}
		}

		for _, actionString := range actionStrings {
			action := ActionFromString(actionString)

			// if not found, try again without pending action in pending for motion only
			if action == ActionNone && d.pendingAction != ActionNone && len(d.pending) > 1 {
				actionStrings, anyStartWith2 := d.keymapper.Get(d.pending[1:], group)
				for _, actionString := range actionStrings {
					a := ActionFromString(actionString)
					if a.IsMotion() {
						action = a
						anyStartWith = anyStartWith2
						break
					}
				}
			}

			// if waitingForMotion is true but the event is not a rune event, reset the action state
			if d.waitingForMotion && event.Key() != tcell.KeyRune {
				d.ResetAction()
				return

				// if waitingForMotion is true and the last motion is waiting for a rune and a rune runner exist for it
			} else if d.waitingForMotion && d.lastMotion.IsWaitingForRune() && d.runeRunner[d.lastMotion] != nil {
				d.runeRunner[d.lastMotion](event.Rune())
				action = d.lastMotion
			}

			// handle operators actions
			// no need to wait for motion action in visual mode
			if action.IsOperator() && (d.mode == visual || d.mode == vline) && action != ActionVisual && action != ActionVisualLine {
				prevMode := d.mode

				if d.mode == vline {
					if d.cursor[0] > d.visualStart[0] || (d.cursor[0] == d.visualStart[0] && d.cursor[1] > d.visualStart[1]) {
						d.cursor, d.visualStart = d.visualStart, d.cursor
					}
					d.cursor[1] = 0
					d.visualStart[1] = len(d.headers) - 1
				}

				d.operatorRunner[action](d.visualStart)
				if d.mode == prevMode {
					d.mode = normal
				}
				d.ResetAction()
				return
			}
			// save operator action in pendingAction, wait for the next motion action
			if action.IsOperator() {
				d.pendingAction = action
				return
			}

			// handle motion actions
			// ignore countless motion (d.g. start of line motion) if pending count is not zero
			if action.IsMotion() && (!action.IsCountlessMotion() || d.pendingCount == 0) &&
				d.motionRunner[action] != nil && (action.IsOperatorlessMotion() || d.pendingAction != ActionNone) {
				m := d.motionRunner[action]()
				if vim.IsAsyncMotion(m) {
					d.lastMotion = action
					return
				}

				if d.operatorRunner[d.pendingAction] != nil {
					d.operatorRunner[d.pendingAction](m)
				}
				d.ResetAction()
				return
			}

			// handle the other action
			if d.actionRunner[action] != nil {
				d.actionRunner[action]()
				d.ResetAction()
				return
			}

			// if there's a keymap that starts with runes in pending, don't reset pending
			if anyStartWith {
				return
			}

			// if it's a digit rune event, save it in pending count
			if isDigit {
				d.pendingCount = d.pendingCount*10 + int(event.Rune()-'0')
				d.pending = d.pending[:len(d.pending)-1]
				return
			}
		}

		d.ResetAction()
	})
}

func (d *Dataviewer) GetUpCursor() [2]int {
	res := [2]int{d.cursor[0] - 1, d.cursor[1]}
	if res[0] < 0 {
		return [2]int{0, d.cursor[1]}
	}
	return res
}

func (d *Dataviewer) GetDownCursor() [2]int {
	res := [2]int{d.cursor[0] + 1, d.cursor[1]}
	if res[0] > len(d.rows) {
		return [2]int{len(d.rows), d.cursor[1]}
	}
	return res
}

func (d *Dataviewer) GetLeftCursor() [2]int {
	res := [2]int{d.cursor[0], d.cursor[1] - 1}
	if res[1] < 0 {
		return [2]int{d.cursor[0], 0}
	}
	return res
}

func (d *Dataviewer) GetRightCursor() [2]int {
	res := [2]int{d.cursor[0], d.cursor[1] + 1}
	if res[1] > len(d.headers)-1 {
		return [2]int{d.cursor[0], len(d.headers) - 1}
	}
	return res
}

func (d *Dataviewer) GetEndOfLineCursor() [2]int {
	return [2]int{d.cursor[0], len(d.headers) - 1}
}

func (d *Dataviewer) GetStartOfLineCursor() [2]int {
	return [2]int{d.cursor[0], 0}
}

func (d *Dataviewer) GetFirstLineCursor() [2]int {
	return [2]int{0, d.cursor[1]}
}

func (d *Dataviewer) GetLastLineCursor() [2]int {
	return [2]int{len(d.rows), d.cursor[1]}
}

func (d *Dataviewer) MoveCursorTo(to [2]int) {
	d.cursor = to
}

func (d *Dataviewer) EnableSearch() [2]int {
	x, y, w, h := d.Box.GetInnerRect()
	se := editor.New(editor.WithKeymapper(d.keymapper)).SetOneLineMode(true)
	se.SetText("", [2]int{0, 0})
	se.SetRect(x, y+h-1, w, 1)
	se.ChangeMode(editor.ModeInsert)
	// se.onDoneFunc = func(s string) {
	// 	d.searchEditor = nil
	// 	d.ResetAction()
	// }
	// se.onExitFunc = func() {
	// 	d.searchEditor = nil
	// 	d.ResetAction()
	// }
	d.searchEditor = se
	d.waitingForMotion = true
	return vim.AsyncMotion
}

func (d *Dataviewer) ResetAction() {
	d.pendingAction = ActionNone
	d.lastMotion = ActionNone
	d.pending = nil
	d.pendingCount = 0
	d.waitingForMotion = false
}
