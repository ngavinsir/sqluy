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

		text                      string
		cameraText                string
		cameraGraphemeIndex       int
		cameraX                   int
		cameraY                   int
		cameraGraphemeIndexMapper map[int]int

		viewModalFunc func(string)
	}
)

func NewEditor() *Editor {
	e := &Editor{
		Box:                       tview.NewBox().SetBorder(true).SetTitle("Editor"),
		text:                      "😊  😊 😊 😊 😊\ntest\nhalo ini siapa\namsok",
		cameraX:                   5,
		cameraY:                   1,
		cameraGraphemeIndexMapper: make(map[int]int),
	}
	e.UpdateCameraText()
	return e
}

func (e *Editor) Draw(screen tcell.Screen) {
	e.Box.DrawForSubclass(screen, e)

	x, y, _, _ := e.Box.GetInnerRect()

	text := e.cameraText
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
	cursor := e.CursorFromGraphemeIndex(e.cameraGraphemeIndex)
	screen.ShowCursor(cursor[0]+x, cursor[1]+y)
}

func (e *Editor) UpdateCameraText() {
	var b strings.Builder

	clear(e.cameraGraphemeIndexMapper)
	text := e.text
	state := -1
	cluster := ""
	boundaries := 0
	lineWidth := 0
	y := 0
	graphemeIndex := 0
	cameraGraphemeIndex := 0
	replacementCount := 0
	for text != "" {
		cluster, text, boundaries, state = uniseg.StepString(text, state)
		clusterWidth := boundaries >> uniseg.ShiftWidth

		if boundaries&uniseg.MaskLine == uniseg.LineMustBreak && text != "" {
			e.cameraGraphemeIndexMapper[cameraGraphemeIndex] = graphemeIndex
			graphemeIndex++

			if y >= e.cameraY {
				cameraGraphemeIndex++
				lineWidth = 0
				b.WriteString(cluster)
			}
			y++
			continue
		}

		// line above camera y, skip
		if y < e.cameraY {
			e.cameraGraphemeIndexMapper[cameraGraphemeIndex] = graphemeIndex
			graphemeIndex++
			continue
		}

		// grapheme before camera x, skip
		if lineWidth < e.cameraX {
			e.cameraGraphemeIndexMapper[cameraGraphemeIndex] = graphemeIndex
			graphemeIndex++
			lineWidth += 1
			if clusterWidth > 1 {
				replacementCount = clusterWidth - 1
				text = strings.Repeat("<", replacementCount) + text
			}
			continue
		}

		e.cameraGraphemeIndexMapper[cameraGraphemeIndex] = graphemeIndex
		if replacementCount <= 0 {
			graphemeIndex++
		} else {
			replacementCount--
		}
		cameraGraphemeIndex++
		b.WriteString(cluster)
		lineWidth += clusterWidth
	}
	e.cameraGraphemeIndexMapper[cameraGraphemeIndex] = graphemeIndex
	e.cameraText = b.String()
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
			e.ReplaceText(text, e.cameraGraphemeIndex, e.cameraGraphemeIndex)
			e.cameraGraphemeIndex++
		case tcell.KeyEnter:
			e.ReplaceText("\n", e.cameraGraphemeIndex, e.cameraGraphemeIndex)
			e.cameraGraphemeIndex++
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			e.ReplaceText("", e.cameraGraphemeIndex-1, e.cameraGraphemeIndex)
			e.cameraGraphemeIndex--
		}
	})
}

func (e *Editor) MoveCursorRight() {
	lines := strings.Split(e.cameraText, "\n")
	graphemeIndex := 0
	for i := 0; i < len(lines); i++ {
		graphemeIndex += uniseg.GraphemeClusterCount(lines[i]) + 1
		if e.cameraGraphemeIndex >= graphemeIndex {
			continue
		}

		if e.cameraGraphemeIndex == graphemeIndex-1 {
			return
		}
	}

	e.cameraGraphemeIndex++
}

func (e *Editor) MoveCursorDown() {
	isTargetLine := false
	curLineX := 0
	lines := strings.Split(e.cameraText, "\n")
	graphemeIndex := 0
	for i := 0; i < len(lines); i++ {
		l := uniseg.GraphemeClusterCount(lines[i]) + 1
		if e.cameraGraphemeIndex >= graphemeIndex+l {
			graphemeIndex += l
			continue
		}

		if !isTargetLine && i >= len(lines)-1 {
			if i < len(strings.Split(e.text, "\n"))-1 {
				e.cameraY++
				e.UpdateCameraText()
			}
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
			e.cameraGraphemeIndex = graphemeIndex
			return
		}

		text := lines[i]
		state := -1
		boundaries := 0
		curLineGraphemeIndex := graphemeIndex
		for curLineGraphemeIndex < e.cameraGraphemeIndex {
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
	lines := strings.Split(e.cameraText, "\n")
	graphemeIndex := uniseg.GraphemeClusterCount(e.cameraText)
	for i := len(lines) - 1; i >= 0; i-- {
		l := uniseg.GraphemeClusterCount(lines[i]) + 1
		if e.cameraGraphemeIndex <= graphemeIndex-l {
			graphemeIndex -= l
			continue
		}

		if !isTargetLine && i == 0 {
			if e.cameraY > 0 {
				e.cameraY--
				e.UpdateCameraText()
			}
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
			e.cameraGraphemeIndex = graphemeIndex
			return
		}

		text := lines[i] + " "
		state := -1
		boundaries := 0
		graphemeIndex -= l - 1
		curLineGraphemeIndex := graphemeIndex
		for curLineGraphemeIndex < e.cameraGraphemeIndex+1 {
			_, text, boundaries, state = uniseg.StepString(text, state)
			curLineX += boundaries >> uniseg.ShiftWidth
			curLineGraphemeIndex++
		}
		isTargetLine = true
	}
}

func (e *Editor) MoveCursorLeft() {
	lines := strings.Split(e.cameraText, "\n")
	graphemeIndex := 0
	for i := 0; i < len(lines); i++ {
		if e.cameraGraphemeIndex == graphemeIndex {
			if e.cameraX > 0 {
				e.cameraX--
				e.UpdateCameraText()
			}
			return
		}

		graphemeIndex += uniseg.GraphemeClusterCount(lines[i]) + 1
		if e.cameraGraphemeIndex >= graphemeIndex {
			continue
		}
	}

	e.cameraGraphemeIndex--
}

func (e *Editor) ReplaceText(s string, fromGraphemeIndex, untilGraphemeIndex int) {
	defer e.UpdateCameraText()

	var b strings.Builder
	// e.viewModalFunc(fmt.Sprintf("camera grapheme index: %d\nfrom grapheme index: %d\nuntil grapheme index: %d\nfrom mapped: %d\nuntil mapped: %d",
	// 	e.cameraGraphemeIndex, fromGraphemeIndex, untilGraphemeIndex, e.cameraGraphemeIndexMapper[fromGraphemeIndex], e.cameraGraphemeIndexMapper[untilGraphemeIndex]))
	fromGraphemeIndex = e.cameraGraphemeIndexMapper[fromGraphemeIndex]
	untilGraphemeIndex = e.cameraGraphemeIndexMapper[untilGraphemeIndex]
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
	lines := strings.Split(e.cameraText, "\n")
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
