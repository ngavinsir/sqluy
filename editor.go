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

		text                 string
		currentGraphemeIndex int // rune index

		viewModalFunc func(string)
	}
)

func NewEditor() *Editor {
	return &Editor{
		Box:  tview.NewBox().SetBorder(true).SetTitle("Editor"),
		text: "😊  😊 😊 😊 😊\ntest\nhalo ini siapa\namsok",
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
		textX += boundaries >> uniseg.ShiftWidth
	}

	screen.SetCursorStyle(tcell.CursorStyleBlinkingBar)
	cursor := e.CursorFromGraphemeIndex(e.currentGraphemeIndex)
	screen.ShowCursor(cursor[0]+x, cursor[1]+y)
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
			e.ReplaceText(text, e.currentGraphemeIndex, e.currentGraphemeIndex)
			e.currentGraphemeIndex++
		case tcell.KeyEnter:
			e.ReplaceText("\n", e.currentGraphemeIndex, e.currentGraphemeIndex)
			e.currentGraphemeIndex++
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			e.ReplaceText("", e.currentGraphemeIndex-1, e.currentGraphemeIndex)
			e.currentGraphemeIndex--
		}
	})
}

func (e *Editor) MoveCursorRight() {
	lines := strings.Split(e.text, "\n")
	graphemeIndex := 0
	for i := 0; i < len(lines); i++ {
		graphemeIndex += uniseg.GraphemeClusterCount(lines[i]) + 1
		if e.currentGraphemeIndex >= graphemeIndex {
			continue
		}

		if e.currentGraphemeIndex == graphemeIndex-1 {
			return
		}
	}

	e.currentGraphemeIndex++
}

func (e *Editor) MoveCursorDown() {
	isTargetLine := false
	curLineX := 0
	lines := strings.Split(e.text, "\n")
	graphemeIndex := 0
	for i := 0; i < len(lines); i++ {
		l := uniseg.GraphemeClusterCount(lines[i]) + 1
		if e.currentGraphemeIndex >= graphemeIndex+l {
			graphemeIndex += l
			continue
		}

		if !isTargetLine && i >= len(lines)-1 {
			return
		}

		if isTargetLine {
			text := lines[i]
			state := -1
			boundaries := 0
			targetLineX := 0
			for text != "" {
				_, text, boundaries, state = uniseg.StepString(text, state)
				x := boundaries >> uniseg.ShiftWidth
				if targetLineX+x > curLineX {
					break
				}
				targetLineX += x
				graphemeIndex++
			}
			e.currentGraphemeIndex = graphemeIndex
			return
		}

		text := lines[i]
		state := -1
		boundaries := 0
		curLineGraphemeIndex := graphemeIndex
		for curLineGraphemeIndex < e.currentGraphemeIndex {
			_, text, boundaries, state = uniseg.StepString(text, state)
			curLineX += boundaries >> uniseg.ShiftWidth
			curLineGraphemeIndex++
		}
		graphemeIndex += l
		isTargetLine = true
	}
}

func (e *Editor) MoveCursorUp() {
	isTargetLine := false
	curLineX := 0
	lines := strings.Split(e.text, "\n")
	graphemeIndex := uniseg.GraphemeClusterCount(e.text)
	for i := len(lines) - 1; i >= 0; i-- {
		l := uniseg.GraphemeClusterCount(lines[i]) + 1
		if e.currentGraphemeIndex <= graphemeIndex-l {
			graphemeIndex -= l
			continue
		}

		if !isTargetLine && i == 0 {
			return
		}

		if isTargetLine {
			text := lines[i]
			state := -1
			boundaries := 0
			targetLineX := 0
			graphemeIndex -= l
			for text != "" {
				_, text, boundaries, state = uniseg.StepString(text, state)
				x := boundaries >> uniseg.ShiftWidth
				if targetLineX+x >= curLineX {
					break
				}
				targetLineX += x
				graphemeIndex++
			}
			e.currentGraphemeIndex = graphemeIndex
			return
		}

		text := lines[i] + " "
		state := -1
		boundaries := 0
		graphemeIndex -= l - 1
		curLineGraphemeIndex := graphemeIndex
		for curLineGraphemeIndex < e.currentGraphemeIndex+1 {
			_, text, boundaries, state = uniseg.StepString(text, state)
			curLineX += boundaries >> uniseg.ShiftWidth
			curLineGraphemeIndex++
		}
		isTargetLine = true
	}
}

func (e *Editor) MoveCursorLeft() {
	lines := strings.Split(e.text, "\n")
	graphemeIndex := 0
	for i := 0; i < len(lines); i++ {
		if e.currentGraphemeIndex == graphemeIndex {
			return
		}

		graphemeIndex += uniseg.GraphemeClusterCount(lines[i]) + 1
		if e.currentGraphemeIndex >= graphemeIndex {
			continue
		}
	}

	e.currentGraphemeIndex--
}

func (e *Editor) ReplaceText(s string, fromGraphemeIndex, untilGraphemeIndex int) {
	var b strings.Builder
	state := -1
	cluster := ""
	graphemeIndex := 0
	text := e.text

	if text == "" {
		e.text = s
		return
	}

	for text != "" {
		if graphemeIndex == fromGraphemeIndex {
			b.WriteString(s)
			graphemeIndex++
			continue
		}

		cluster, text, _, state = uniseg.StepString(text, state)

		if graphemeIndex >= fromGraphemeIndex && graphemeIndex <= untilGraphemeIndex {
			graphemeIndex++
			continue
		}

		b.WriteString(cluster)
		graphemeIndex++
	}
	if graphemeIndex == fromGraphemeIndex {
		b.WriteString(s)
	}

	e.text = b.String()
}

func (e *Editor) CursorFromGraphemeIndex(graphemeIndex int) [2]int {
	lines := strings.Split(e.text, "\n")
	for i := 0; i < len(lines); i++ {
		l := uniseg.GraphemeClusterCount(lines[i]) + 1
		if graphemeIndex < l {
			x := 0
			text := lines[i]
			state := -1
			boundaries := 0
			for graphemeIndex != 0 {
				_, text, boundaries, state = uniseg.StepString(text, state)
				x += boundaries >> uniseg.ShiftWidth
				graphemeIndex--
			}
			return [2]int{x, i}
		}
		graphemeIndex -= l
	}
	return [2]int{0, len(lines)}
}
