package main

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/rivo/uniseg"
)

type (
	Editor struct {
		*tview.Box

		text   string
		cursor [2]int
	}
)

func NewEditor() *Editor {
	return &Editor{
		Box:  tview.NewBox().SetBorder(true).SetTitle("Editor"),
		text: "test\nhalo ini siapa\namsok",
	}
}

func (e *Editor) Draw(screen tcell.Screen) {
	e.Box.DrawForSubclass(screen, e)

	x, y, _, _ := e.Box.GetInnerRect()

	text := e.text
	state := -1
	cluster := ""
	textX, textY := x, y
	boundaries := 0
	for text != "" {
		cluster, text, boundaries, state = uniseg.StepString(text, state)

		if boundaries&uniseg.MaskLine == uniseg.LineMustBreak && text != "" {
			textY++
			textX = x
			continue
		}

		runes := []rune(cluster)
		screen.SetContent(textX, textY, runes[0], runes[1:], tcell.StyleDefault.Foreground(tcell.ColorRed))
		textX++

	}

	screen.SetCursorStyle(tcell.CursorStyleBlinkingBar)
	screen.ShowCursor(e.cursor[0]+x, e.cursor[1]+y)
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
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			didDeleteNewLine := e.cursor[1] > 0 && e.cursor[0] == 0
			prevLineLastX := 0
			if didDeleteNewLine {
				prevLineLastX = len(strings.Split(e.text, "\n")[e.cursor[1]-1])
			}

			e.ReplaceText("", [2]int{e.cursor[0] - 1, e.cursor[1]}, e.cursor)

			if didDeleteNewLine {
				e.MoveCursorUp()
				e.cursor[0] = prevLineLastX
			} else {
				e.MoveCursorLeft()
			}
		}
	})
}

func (e *Editor) MoveCursorRight() {
	e.cursor[0]++
	curLineLastX := len(strings.Split(e.text, "\n")[e.cursor[1]])
	if e.cursor[0] > curLineLastX {
		e.cursor[0] = curLineLastX
	}
}

func (e *Editor) MoveCursorDown() {
	e.cursor[1]++
	if e.cursor[1] >= len(strings.Split(e.text, "\n")) {
		e.cursor[1] = len(strings.Split(e.text, "\n")) - 1
	}
	if e.cursor[0] > 0 {
		e.MoveCursorLeft()
		e.MoveCursorRight()
	}
}

func (e *Editor) MoveCursorUp() {
	e.cursor[1]--
	if e.cursor[1] < 0 {
		e.cursor[1] = 0
	}
	if e.cursor[0] > 0 {
		e.MoveCursorLeft()
		e.MoveCursorRight()
	}
}

func (e *Editor) MoveCursorLeft() {
	e.cursor[0]--
	if e.cursor[0] < 0 {
		e.cursor[0] = 0
	}
}

func (e *Editor) MoveCursorEnd() {
	curLineLastX := len(strings.Split(e.text, "\n")[e.cursor[1]])
	e.cursor[0] = curLineLastX
}

func (e *Editor) ReplaceText(text string, from, until [2]int) {
	var b strings.Builder
	textRunes := []rune(e.text)

	leftUntil := e.RuneIndexFromCursor([2]int{e.cursor[0] - (until[0] - from[0]), e.cursor[1]})
	if leftUntil > 0 {
		b.WriteString(string(textRunes[:leftUntil]))
	}

	b.WriteString(text)

	rightFrom := e.RuneIndexFromCursor(e.cursor)
	if rightFrom < len(e.text) {
		b.WriteString(string(textRunes[rightFrom:]))
	}

	e.text = b.String()
}

func (e *Editor) RuneIndexFromCursor(cursor [2]int) int {
	if cursor[1] < 1 {
		return cursor[0]
	}

	lines := strings.Split(e.text, "\n")
	index := 0
	for i := 0; i < len(lines); i++ {
		if i == cursor[1] {
			return index + cursor[0]
		}

		index += len([]rune(lines[i])) + 1
	}

	return index
}
