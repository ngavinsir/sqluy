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
		text: "test\nhalo\nasok",
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

		if boundaries&uniseg.MaskLine == uniseg.LineMustBreak {
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
		case tcell.KeyRune:
			textRunes := []rune(e.text)
			r := event.Rune()
			e.text = string(textRunes[:e.cursor[0]]) + string(r) + string(textRunes[e.cursor[0]:])
			e.MoveCursorRight()
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			var b strings.Builder
			textRunes := []rune(e.text)
			leftUntil := e.cursor[0] - 1
			if leftUntil > 0 {
				b.WriteString(string(textRunes[:leftUntil]))
			}
			rightFrom := e.cursor[0]
			if rightFrom < len(e.text) {
				b.WriteString(string(textRunes[rightFrom:]))
			}
			e.text = b.String()
			e.MoveCursorLeft()
		}
	})
}

func (e *Editor) MoveCursorRight() {
	e.cursor[0]++
	if e.cursor[0] > len(e.text) {
		e.cursor[0] = len(e.text)
	}
}

func (e *Editor) MoveCursorLeft() {
	e.cursor[0]--
	if e.cursor[0] < 0 {
		e.cursor[0] = 0
	}
}
