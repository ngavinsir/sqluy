package main

import (
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

		viewModalFunc func(string)
	}
)

func NewEditor() *Editor {
	e := &Editor{
		Box: tview.NewBox().SetBorder(true).SetTitle("Editor"),
	}
	// e.SetText("abc\nde\nrsitenrstnrsyu\nrstn\na\nb\n\n\na")
	e.SetText("😊😊  😊 😊 😊 😊\ntest\nhalo ini siapa\namsok", [2]int{0, 0})
	// panic(fmt.Sprintf("%+v\n", e.cameraGraphemeIndexMapper))
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

	x, y, _, _ := e.Box.GetInnerRect()

	textX := x
	textY := y
	for _, spans := range e.spansPerLines {
		for _, span := range spans {
			if span.runes == nil {
				break
			}
			runes := span.runes
			screen.SetContent(textX, textY, runes[0], runes[1:], tcell.StyleDefault.Foreground(tcell.ColorRed))
			textX += int(span.width)
		}
		textY++
		textX = x
	}

	cursorX := x
	for _, span := range e.spansPerLines[e.cursor[0]][:e.cursor[1]] {
		cursorX += span.width
	}
	screen.SetCursorStyle(tcell.CursorStyleBlinkingBar)
	screen.ShowCursor(cursorX, e.cursor[0]+y)
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

	// panic(errors.New(b.String()))
	e.SetText(b.String(), e.cursor)
}
