package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/rivo/uniseg"
)

type (
	span struct {
		runes []rune
		width int
	}

	Editor struct {
		viewModalFunc func(string)
		*tview.Box
		text          string
		spansPerLines [][]span
		cursor        [2]int
		offsets       [2]int
		tabSize       int
		mode          mode
	}
)

type mode uint8

const (
	normal mode = iota
	insert
)

func (m mode) String() string {
	switch m {
	case insert:
		return "INSERT"
	default:
		return "NORMAL"
	}
}

func NewEditor() *Editor {
	e := &Editor{
		tabSize: 4,
		Box:     tview.NewBox().SetBorder(true).SetTitle("Editor"),
	}
	// e.SetText("😊😊  😊 😊 😊 😊\n  test\nhalo ini siapa\namsok", [2]int{3, 0})
	e.SetText(`
	package main

	import (
		"fmt"
		"strconv"
		"strings"

		"github.com/gdamore/tcell/v2"
		"github.com/rivo/tview"
		"github.com/rivo/uniseg"
	)

	type (
		span struct {
			runes []rune
			width int
		}

		Editor struct {
			*tview.Box

			text          string
			spansPerLines [][]span
			cursor        [2]int // row, grapheme index

			offsets [2]int // row, column offsets

			viewModalFunc func(string)
		}
	)

	func NewEditor() *Editor {
		e := &Editor{
			Box: tview.NewBox().SetBorder(true).SetTitle("Editor"),
		}
		e.SetText("😊😊  😊 😊 😊 😊\ntest\nhalo ini siapa\namsok", [2]int{3, 0})
		return e
	}

	func (e *Editor) SetText(text string, cursor [2]int) *Editor {
		clear(e.spansPerLines)

		lines := strings.Split(text, "\n")
		e.spansPerLines = make([][]span, len(lines))
		e.cursor = cursor
		e.text = text

		for i, line := range lines {
			text = line
			spans := make([]span, uniseg.GraphemeClusterCount(text)+1)
			state := -1
			cluster := ""
			boundaries := 0
			j := 0
			for text != "" {
				cluster, text, boundaries, state = uniseg.StepString(text, state)

				span := span{
					width: boundaries >> uniseg.ShiftWidth,
					runes: []rune(cluster),
				}
				spans[j] = span
				j++
			}
			spans[j] = span{runes: nil, width: 1}
			e.spansPerLines[i] = spans
		}
		// panic(errors.New(fmt.Sprintf("%+v\n", e.spansPerLines[0])))

		return e
	}

	func (e *Editor) Draw(screen tcell.Screen) {
		e.Box.DrawForSubclass(screen, e)

		x, y, w, h := e.Box.GetInnerRect()

		// print line numbers
		lineNumberDigit := len(strconv.Itoa(len(e.spansPerLines)))
		lineNumberWidth := lineNumberDigit + 1
		textY := y
		lastLine := e.offsets[0] + h
		if lastLine > len(e.spansPerLines) {
			lastLine = len(e.spansPerLines)
		}
		for i, _ := range e.spansPerLines[e.offsets[0]:lastLine] {
			lineNumberText := []rune(fmt.Sprintf("%*d", lineNumberDigit, i+e.offsets[0]+1))
			screen.SetContent(x, textY, lineNumberText[0], lineNumberText[1:], tcell.StyleDefault.Foreground(tcell.ColorSlateGray))
			screen.SetContent(x+lineNumberDigit, textY, []rune(" ")[0], nil, tcell.StyleDefault)
			textY++
		}
		x += lineNumberWidth
		w -= lineNumberWidth

		// fix offsets position so the cursor is visible
		// cursor is above row offset, set row offset to cursor row
		if e.cursor[0] < e.offsets[0] {
			e.offsets[0] = e.cursor[0]
		}
		// cursor is below row offset
		if e.cursor[0] >= e.offsets[0]+h {
			e.offsets[0] = e.cursor[0] - h + 1
		}
		// adjust offset so there's no empty line
		if e.offsets[0]+h > len(e.spansPerLines) {
			e.offsets[0] = len(e.spansPerLines) - h
			if e.offsets[0] < 0 {
				e.offsets[0] = 0
			}
		}

		cursorX := 0
		for _, span := range e.spansPerLines[e.cursor[0]][:e.cursor[1]] {
			cursorX += span.width
		}
		// cursor is before column offset
		if cursorX < e.offsets[1] {
			e.offsets[1] = cursorX - 1
			if e.offsets[1] < 0 {
				e.offsets[1] = 0
			}
		}
		// cursor is after column offset
		if cursorX > e.offsets[1]+w {
			e.offsets[1] = cursorX - w + 1
		}

		textX := x
		textY = y
		lastLine = e.offsets[0] + h
		if lastLine > len(e.spansPerLines) {
			lastLine = len(e.spansPerLines)
		}
		for _, spans := range e.spansPerLines[e.offsets[0]:lastLine] {
			for _, span := range spans {
				// skip drawing end line sentinel
				if span.runes == nil {
					break
				}
				// skip grapheme completely hidden on the left
				if textX+span.width <= x+e.offsets[1] {
					textX += span.width
					continue
				}
				// skip grapheme completely hidden on the right
				if textX >= x+e.offsets[1]+w {
					break
				}

				runes := span.runes
				width := span.width
				// replace too wide grapheme on the left edge
				if textX < x+e.offsets[1] && textX+width > x+e.offsets[1] {
					c := textX + width - (x + e.offsets[1])
					runes = []rune(strings.Repeat("<", c))
					textX += width - c
					width = c
				} else if textX+width > x+e.offsets[1]+w { // too wide grapheme on the right edge
					c := (x + e.offsets[1] + w) - textX
					runes = []rune(strings.Repeat(">", c))
					width = c
				}
				screen.SetContent(textX-e.offsets[1], textY, runes[0], runes[1:], tcell.StyleDefault.Foreground(tcell.ColorRed))
				textX += width
			}
			textY++
			textX = x
		}

		screen.SetCursorStyle(tcell.CursorStyleBlinkingBar)
		screen.ShowCursor(cursorX+x-e.offsets[1], e.cursor[0]+y-e.offsets[0])
	}

	func (e *Editor) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		return e.Box.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
			switch key := event.Key(); key {
			case tcell.KeyLeft:
				e.MoveCursorLeft()
				// e.viewModalFunc(strconv.Itoa(e.cameraGraphemeIndex))
			case tcell.KeyRight:
				e.MoveCursorRight()
				// e.viewModalFunc(strconv.Itoa(e.cameraGraphemeIndex))
			case tcell.KeyDown:
				e.MoveCursorDown()
				// e.viewModalFunc(strconv.Itoa(e.cameraGraphemeIndex))
			case tcell.KeyUp:
				e.MoveCursorUp()
				// e.viewModalFunc(strconv.Itoa(e.cameraGraphemeIndex))
			case tcell.KeyRune:
				text := string(event.Rune())
				e.ReplaceText(text, e.cursor, e.cursor)
				e.MoveCursorRight()
			case tcell.KeyEnter:
				e.ReplaceText("\n", e.cursor, e.cursor)
				e.MoveCursorDown()
				e.cursor[1] = 0
			case tcell.KeyBackspace, tcell.KeyBackspace2:
				if e.cursor[0] == 0 && e.cursor[1] == 0 {
					return
				}

				from := [2]int{e.cursor[0], e.cursor[1] - 1}
				until := e.cursor
				if e.cursor[1] == 0 {
					aboveRow := e.cursor[0] - 1
					from = [2]int{aboveRow, len(e.spansPerLines[aboveRow]) - 1}
					until = [2]int{e.cursor[0], 0}
				}
				e.ReplaceText("", from, until)
				e.cursor = from
				// panic(errors.New(fmt.Sprintf("cursor: %+v\noffset: %+v\n", e.cursor, e.offsets)))
			}
		})
	}

	func (e *Editor) MoveCursorRight() {
		if e.cursor[1] >= len(e.spansPerLines[e.cursor[0]])-1 {
			return
		}

		e.cursor[1]++
	}

	func (e *Editor) MoveCursorLeft() {
		if e.cursor[1] < 1 {
			return
		}

		e.cursor[1]--
	}

	func (e *Editor) MoveCursorDown() {
		if e.cursor[0] >= len(e.spansPerLines)-1 {
			return
		}

		currentRowWidth := 0
		for _, span := range e.spansPerLines[e.cursor[0]][:e.cursor[1]] {
			currentRowWidth += span.width
		}

		belowRowX := 0
		belowRowWidth := 0
		for _, span := range e.spansPerLines[e.cursor[0]+1] {
			if span.runes == nil {
				break
			}
			if belowRowWidth+span.width > currentRowWidth {
				break
			}
			belowRowX++
			belowRowWidth += span.width
		}

		e.cursor[0]++
		e.cursor[1] = belowRowX
	}

	func (e *Editor) MoveCursorUp() {
		if e.cursor[0] < 1 {
			return
		}

		currentRowWidth := 0
		for _, span := range e.spansPerLines[e.cursor[0]][:e.cursor[1]] {
			currentRowWidth += span.width
		}

		aboveRowX := 0
		aboveRowWidth := 0
		for _, span := range e.spansPerLines[e.cursor[0]-1] {
			if span.runes == nil {
				break
			}
			if aboveRowWidth+span.width > currentRowWidth {
				break
			}
			aboveRowX++
			aboveRowWidth += span.width
		}

		e.cursor[0]--
		e.cursor[1] = aboveRowX
	}

	func (e *Editor) ReplaceText(s string, from, until [2]int) {
		if from[0] > until[0] || from[0] == until[0] && from[1] > until[1] {
			from, until = until, from
		}

		var b strings.Builder
		lines := strings.Split(e.text, "\n")

		// write left
		for _, l := range lines[:from[0]] {
			b.WriteString(l + "\n")
		}

		// write new text
		// from row
		for _, span := range e.spansPerLines[from[0]][:from[1]] {
			b.WriteString(string(span.runes))
		}
		// new text
		b.WriteString(s)
		// until row
		for _, span := range e.spansPerLines[until[0]][until[1]:] {
			b.WriteString(string(span.runes))
		}
		if until[0] < len(lines)-1 {
			b.WriteString("\n")
		}

		// write right
		for i, l := range lines {
			if i < until[0]+1 {
				continue
			}

			b.WriteString(l)
			if i < len(lines)-1 {
				b.WriteString("\n")
			}
		}

		e.SetText(b.String(), e.cursor)
	}
	    `, [2]int{0, 0})
	return e
}

func (e *Editor) SetText(text string, cursor [2]int) *Editor {
	clear(e.spansPerLines)

	lines := strings.Split(text, "\n")
	e.spansPerLines = make([][]span, len(lines))
	e.cursor = cursor
	e.text = text

	for i, line := range lines {
		text = line
		spans := make([]span, uniseg.GraphemeClusterCount(text)+1)
		state := -1
		cluster := ""
		boundaries := 0
		j := 0
		for text != "" {
			cluster, text, boundaries, state = uniseg.StepString(text, state)

			width := boundaries >> uniseg.ShiftWidth
			if cluster == "\t" {
				width = e.tabSize
			}
			span := span{
				width: width,
				runes: []rune(cluster),
			}
			spans[j] = span
			j++
		}
		spans[j] = span{runes: nil, width: 1}
		e.spansPerLines[i] = spans
	}
	// panic(errors.New(fmt.Sprintf("%+v\n", e.spansPerLines[0])))

	return e
}

func (e *Editor) Draw(screen tcell.Screen) {
	e.Box.DrawForSubclass(screen, e)

	x, y, w, h := e.Box.GetInnerRect()

	// print mode
	tview.Print(screen, fmt.Sprintf("%s mode", e.mode.String()), x, y+h-1, w, tview.AlignLeft, tcell.ColorDarkGreen)
	h--

	// fix offsets position so the cursor is visible
	// cursor is above row offset, set row offset to cursor row
	if e.cursor[0] < e.offsets[0] {
		e.offsets[0] = e.cursor[0]
	}
	// cursor is below row offset
	if e.cursor[0] >= e.offsets[0]+h {
		e.offsets[0] = e.cursor[0] - h + 1
	}
	// adjust offset so there's no empty line
	if e.offsets[0]+h > len(e.spansPerLines) {
		e.offsets[0] = len(e.spansPerLines) - h
		if e.offsets[0] < 0 {
			e.offsets[0] = 0
		}
	}

	cursorX := 0
	for _, span := range e.spansPerLines[e.cursor[0]][:e.cursor[1]] {
		cursorX += span.width
	}
	// cursor is before column offset
	if cursorX < e.offsets[1] {
		e.offsets[1] = cursorX - 1
		if e.offsets[1] < 0 {
			e.offsets[1] = 0
		}
	}
	// print line numbers
	lineNumberDigit := len(strconv.Itoa(len(e.spansPerLines)))
	lineNumberWidth := lineNumberDigit + 1
	textX := x
	textY := y
	lastLine := e.offsets[0] + h
	if lastLine > len(e.spansPerLines) {
		lastLine = len(e.spansPerLines)
	}
	for i, _ := range e.spansPerLines[e.offsets[0]:lastLine] {
		lineNumber := i + e.offsets[0] + 1
		lineNumberText := fmt.Sprintf("%*d", lineNumberDigit, lineNumber)
		state := -1
		cluster := ""
		for lineNumberText != "" {
			cluster, lineNumberText, _, state = uniseg.StepString(lineNumberText, state)
			runes := []rune(cluster)
			screen.SetContent(textX, textY, runes[0], runes[1:], tcell.StyleDefault.Foreground(tcell.ColorSlateGray))
			textX++
		}
		textY++
		textX = x
	}
	x += lineNumberWidth
	w -= lineNumberWidth
	// panic(errors.New(strconv.Itoa(w)))

	// cursor is after column offset
	if cursorX > e.offsets[1]+w {
		e.offsets[1] = cursorX - w + 1
	}

	textX = x
	textY = y
	lastLine = e.offsets[0] + h
	if lastLine > len(e.spansPerLines) {
		lastLine = len(e.spansPerLines)
	}
	for _, spans := range e.spansPerLines[e.offsets[0]:lastLine] {
		for _, span := range spans {
			// skip drawing end line sentinel
			if span.runes == nil {
				// panic(textX)
				break
			}
			// skip grapheme completely hidden on the left
			if textX+span.width <= x+e.offsets[1] {
				textX += span.width
				continue
			}
			// skip grapheme completely hidden on the right
			if textX >= x+e.offsets[1]+w {
				break
			}

			runes := span.runes
			width := span.width
			// replace too wide grapheme on the left edge that's not a tab
			if textX < x+e.offsets[1] && textX+width > x+e.offsets[1] && runes[0] != '\t' {
				c := textX + width - (x + e.offsets[1])
				runes = []rune(strings.Repeat("<", c))
				textX += width - c
				width = c
			} else if textX+width > x+e.offsets[1]+w && runes[0] != '\t' { // too wide grapheme on the right edge that's not a tab
				c := (x + e.offsets[1] + w) - textX
				runes = []rune(strings.Repeat(">", c))
				width = c
			}
			if runes[0] != '\t' {
				screen.SetContent(textX-e.offsets[1], textY, runes[0], runes[1:], tcell.StyleDefault.Foreground(tcell.ColorRed))
			}
			textX += width
		}
		textY++
		textX = x
	}

	screen.SetCursorStyle(tcell.CursorStyleBlinkingBar)
	screen.ShowCursor(cursorX+x-e.offsets[1], e.cursor[0]+y-e.offsets[0])
}

func (e *Editor) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return e.Box.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		switch key := event.Key(); key {
		case tcell.KeyLeft:
			e.MoveCursorLeft()
		case tcell.KeyRight:
			e.MoveCursorRight()
		case tcell.KeyDown:
			e.MoveCursorDown()
		case tcell.KeyUp:
			e.MoveCursorUp()
		case tcell.KeyRune:
			text := string(event.Rune())
			e.ReplaceText(text, e.cursor, e.cursor)
			e.MoveCursorRight()
		case tcell.KeyEnter:
			e.ReplaceText("\n", e.cursor, e.cursor)
			e.MoveCursorDown()
			e.cursor[1] = 0
		case tcell.KeyTab:
			e.ReplaceText("\t", e.cursor, e.cursor)
			e.MoveCursorRight()
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			if e.cursor[0] == 0 && e.cursor[1] == 0 {
				return
			}

			from := [2]int{e.cursor[0], e.cursor[1] - 1}
			until := e.cursor
			if e.cursor[1] == 0 {
				aboveRow := e.cursor[0] - 1
				from = [2]int{aboveRow, len(e.spansPerLines[aboveRow]) - 1}
				until = [2]int{e.cursor[0], 0}
			}
			e.ReplaceText("", from, until)
			e.cursor = from
			// panic(errors.New(fmt.Sprintf("cursor: %+v\noffset: %+v\n", e.cursor, e.offsets)))
		}
	})
}

func (e *Editor) MoveCursorRight() {
	if e.cursor[1] >= len(e.spansPerLines[e.cursor[0]])-1 {
		return
	}

	e.cursor[1]++
}

func (e *Editor) MoveCursorLeft() {
	if e.cursor[1] < 1 {
		return
	}

	e.cursor[1]--
}

func (e *Editor) MoveCursorDown() {
	if e.cursor[0] >= len(e.spansPerLines)-1 {
		return
	}

	currentRowWidth := 0
	for _, span := range e.spansPerLines[e.cursor[0]][:e.cursor[1]] {
		currentRowWidth += span.width
	}

	belowRowX := 0
	belowRowWidth := 0
	for _, span := range e.spansPerLines[e.cursor[0]+1] {
		if span.runes == nil {
			break
		}
		if belowRowWidth+span.width > currentRowWidth {
			break
		}
		belowRowX++
		belowRowWidth += span.width
	}

	e.cursor[0]++
	e.cursor[1] = belowRowX
}

func (e *Editor) MoveCursorUp() {
	if e.cursor[0] < 1 {
		return
	}

	currentRowWidth := 0
	for _, span := range e.spansPerLines[e.cursor[0]][:e.cursor[1]] {
		currentRowWidth += span.width
	}

	aboveRowX := 0
	aboveRowWidth := 0
	for _, span := range e.spansPerLines[e.cursor[0]-1] {
		if span.runes == nil {
			break
		}
		if aboveRowWidth+span.width > currentRowWidth {
			break
		}
		aboveRowX++
		aboveRowWidth += span.width
	}

	e.cursor[0]--
	e.cursor[1] = aboveRowX
}

func (e *Editor) ReplaceText(s string, from, until [2]int) {
	if from[0] > until[0] || from[0] == until[0] && from[1] > until[1] {
		from, until = until, from
	}

	var b strings.Builder
	lines := strings.Split(e.text, "\n")

	// write left
	for _, l := range lines[:from[0]] {
		b.WriteString(l + "\n")
	}

	// write new text
	// from row
	for _, span := range e.spansPerLines[from[0]][:from[1]] {
		b.WriteString(string(span.runes))
	}
	// new text
	b.WriteString(s)
	// until row
	for _, span := range e.spansPerLines[until[0]][until[1]:] {
		b.WriteString(string(span.runes))
	}
	if until[0] < len(lines)-1 {
		b.WriteString("\n")
	}

	// write right
	for i, l := range lines {
		if i < until[0]+1 {
			continue
		}

		b.WriteString(l)
		if i < len(lines)-1 {
			b.WriteString("\n")
		}
	}

	e.SetText(b.String(), e.cursor)
}
