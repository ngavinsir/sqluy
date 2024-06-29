package editor

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/rivo/uniseg"
)

type (
	keymapper interface {
		Get(keys []string, group string) (string, bool)
	}

	undoStackItem struct {
		text   string
		cursor [2]int
	}

	span struct {
		runes      []rune
		width      int // printable width
		bytesWidth int
	}

	decoration struct {
		style tcell.Style
		text  string
	}

	decorator func(row, col int) (decoration, bool)

	Editor struct {
		keymapper     keymapper
		screen        tcell.Screen
		viewModalFunc func(string)
		onDoneFunc    func(string)
		onExitFunc    func()
		*tview.Box
		searchEditor  *Editor
		actionRunner  map[Action]func()
		motionIndexes map[string][][3]int
		text          string
		pending       []string
		spansPerLines [][]span
		undoStack     []undoStackItem
		cursor        [2]int
		offsets       [2]int
		tabSize       int
		editCount     uint64
		undoOffset    int
		mode          mode
		oneLineMode   bool
		decorators    []decorator
	}
)

func New(km keymapper) *Editor {
	e := &Editor{
		tabSize:   4,
		Box:       tview.NewBox(),
		keymapper: km,
	}
	// e.SetText("amsok", [2]int{0, 0})
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

	e.actionRunner = map[Action]func(){
		ActionMoveLeft:     e.MoveCursorLeft,
		ActionMoveUp:       e.MoveCursorUp,
		ActionMoveRight:    e.MoveCursorRight,
		ActionMoveDown:     e.MoveCursorDown,
		ActionDone:         e.Done,
		ActionExit:         e.Exit,
		ActionEnableSearch: e.EnableSearch,
		ActionInsert: func() {
			e.ChangeMode(insert)
		},
		ActionRedo:                   e.Redo,
		ActionUndo:                   e.Undo,
		ActionMoveHalfPageDown:       e.MoveCursorHalfPageDown,
		ActionMoveHalfPageUp:         e.MoveCursorHalfPageUp,
		ActionDeleteUnderCursor:      e.DeleteUnderCursor,
		ActionInsertAfter:            e.InsertAfter,
		ActionInsertEndOfLine:        e.InsertEndOfLine,
		ActionMoveEndOfLine:          e.MoveCursorEndOfLine,
		ActionMoveStartOfLine:        e.MoveCursorStartOfLine,
		ActionMoveFirstNonWhitespace: e.MoveCursorFirstNonWhitespace,
		ActionInsertBelow:            e.InsertBelow,
		ActionInsertAbove:            e.InsertAbove,
		ActionChangeUntilEndOfLine:   e.ChangeUntilEndOfLine,
		ActionDeleteUntilEndOfLine:   e.DeleteUntilEndOfLine,
		ActionDeleteLine:             e.DeleteLine,
		ActionReplace: func() {
			e.ChangeMode(replace)
		},
		ActionMoveLastLine:  e.MoveCursorLastLine,
		ActionMoveFirstLine: e.MoveCursorFirstLine,
		ActionMoveEndOfWord: func() {
			e.MoveMotion("e", 1)
		},
		ActionMoveStartOfWord: func() {
			e.MoveMotion("w", 1)
		},
		ActionMoveBackStartOfWord: func() {
			e.MoveMotion("w", -1)
		},
		ActionMoveBackEndOfWord: func() {
			e.MoveMotion("e", -1)
		},
		ActionMoveNextSearch: func() {
			e.MoveMotion("n", 1)
		},
		ActionMovePrevSearch: func() {
			e.MoveMotion("n", -1)
		},
	}

	e.decorators = []decorator{
		// search highlighter
		func(row, col int) (decoration, bool) {
			if e.motionIndexes["n"] == nil {
				return decoration{}, false
			}

			style := tcell.StyleDefault.Background(tview.Styles.MoreContrastBackgroundColor).Foreground(tview.Styles.PrimitiveBackgroundColor)
			for _, idx := range e.motionIndexes["n"] {
				if idx[0] != row {
					continue
				}

				if col >= idx[1] && col < idx[2] {
					return decoration{style: style, text: ""}, true
				}
			}

			return decoration{}, false
		},
	}

	return e
}

func (e *Editor) SetOneLineMode(b bool) *Editor {
	e.oneLineMode = b
	if !b {
		e.Box.SetBorder(true).SetTitle("Editor")
	}
	return e
}

func (e *Editor) SetViewModalFunc(f func(string)) *Editor {
	e.viewModalFunc = f
	return e
}

func (e *Editor) SetText(text string, cursor [2]int) *Editor {
	e.editCount++
	clear(e.spansPerLines)

	lines := strings.Split(text, "\n")
	if e.oneLineMode {
		lines = lines[:1]
	}
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
			_, bytesWidth := utf8.DecodeRuneInString(cluster)
			span := span{
				width:      width,
				runes:      []rune(cluster),
				bytesWidth: bytesWidth,
			}
			spans[j] = span
			j++
		}
		spans[j] = span{runes: nil, width: 1}
		e.spansPerLines[i] = spans
	}

	e.motionIndexes = make(map[string][][3]int)
	go e.buildMotionwIndexes(e.editCount)
	go e.buildMotioneIndexes(e.editCount)

	return e
}

func (e *Editor) buildSearchIndexes(query string) bool {
	foundMatches := false
	rg := regexp.MustCompile(query)

	var indexes [][3]int
	for i, line := range strings.Split(e.text, "\n") {
		if len(line) == 0 {
			continue
		}

		bytesWidthSum := 0
		for _, s := range e.spansPerLines[i] {
			bytesWidthSum += s.bytesWidth
		}
		mapper := make([]int, bytesWidthSum)
		mapperIdx := 0
		for i, s := range e.spansPerLines[i] {
			for j := range s.bytesWidth {
				mapper[mapperIdx+j] = i
			}
			mapperIdx += s.bytesWidth
		}

		matches := rg.FindAllStringSubmatchIndex(line, -1)

		for _, m := range matches {
			if len(m) == 0 {
				continue
			}

			foundMatches = true
			indexes = append(indexes, [3]int{i, mapper[m[0]], mapper[m[1]]})
		}
	}

	e.motionIndexes["n"] = indexes
	return foundMatches
}

func (e *Editor) buildMotionwIndexes(editCount uint64) {
	defer func() {
		if r := recover(); r != nil && e.screen != nil {
			e.screen.Fini()
			panic(r)
		}
	}()

	rgOne := regexp.MustCompile(`(?:^|[^a-zA-Z0-9_À-ÿ])([a-zA-Z0-9_À-ÿ])`)
	rgTwo := regexp.MustCompile(`(?:^|[a-zA-Z0-9_À-ÿ\s])([^a-zA-Z0-9_À-ÿ\s])`)

	var indexes [][3]int
	for i, line := range strings.Split(e.text, "\n") {
		if e.editCount > editCount {
			return
		}
		if len(line) == 0 {
			continue
		}

		bytesWidthSum := 0
		for _, s := range e.spansPerLines[i] {
			bytesWidthSum += s.bytesWidth
		}
		mapper := make([]int, bytesWidthSum)
		mapperIdx := 0
		for i, s := range e.spansPerLines[i] {
			for j := range s.bytesWidth {
				mapper[mapperIdx+j] = i
			}
			mapperIdx += s.bytesWidth
		}

		matchesOne := rgOne.FindAllStringSubmatchIndex(line, -1)
		matchesTwo := rgTwo.FindAllStringSubmatchIndex(line, -1)

		for _, m := range matchesOne {
			if len(m) < 4 || m[2] >= m[3] {
				continue
			}

			indexes = append(indexes, [3]int{i, mapper[m[2]], mapper[m[3]-1]})
		}
		for _, m := range matchesTwo {
			if len(m) < 4 || m[2] >= m[3] {
				continue
			}

			indexes = append(indexes, [3]int{i, mapper[m[2]], mapper[m[3]-1]})
		}
	}
	sort.Slice(indexes, func(i, j int) bool {
		return indexes[i][0] < indexes[j][0] || (indexes[i][0] == indexes[j][0] && indexes[i][1] < indexes[j][1])
	})

	if e.editCount > editCount {
		return
	}
	e.motionIndexes["w"] = indexes
}

func (e *Editor) buildMotioneIndexes(editCount uint64) {
	defer func() {
		if r := recover(); r != nil && e.screen != nil {
			e.screen.Fini()
			panic(r)
		}
	}()

	rgOne := regexp.MustCompile(`([^{a-zA-Z0-9_À-ÿ}\s])[{a-zA-Z0-9_À-ÿ}\s]`)
	rgTwo := regexp.MustCompile(`([{a-zA-Z0-9_À-ÿ}])(?:[^{a-zA-Z0-9_À-ÿ}]|$)`)

	var indexes [][3]int
	for i, line := range strings.Split(e.text, "\n") {
		if e.editCount > editCount {
			return
		}
		if len(line) == 0 {
			continue
		}

		bytesWidthSum := 0
		for _, s := range e.spansPerLines[i] {
			bytesWidthSum += s.bytesWidth
		}
		mapper := make([]int, bytesWidthSum)
		mapperIdx := 0
		for i, s := range e.spansPerLines[i] {
			for j := range s.bytesWidth {
				mapper[mapperIdx+j] = i
			}
			mapperIdx += s.bytesWidth
		}

		matchesOne := rgOne.FindAllStringSubmatchIndex(line, -1)
		matchesTwo := rgTwo.FindAllStringSubmatchIndex(line, -1)

		for _, m := range matchesOne {
			if len(m) < 4 || m[2] >= m[3] {
				continue
			}

			indexes = append(indexes, [3]int{i, mapper[m[2]], mapper[m[3]-1]})
		}
		for _, m := range matchesTwo {
			if len(m) < 4 || m[2] >= m[3] {
				continue
			}

			indexes = append(indexes, [3]int{i, mapper[m[2]], mapper[m[3]-1]})
		}
	}
	sort.Slice(indexes, func(i, j int) bool {
		return indexes[i][0] < indexes[j][0] || (indexes[i][0] == indexes[j][0] && indexes[i][1] < indexes[j][1])
	})

	if e.editCount > editCount {
		return
	}
	e.motionIndexes["e"] = indexes
	// panic(fmt.Sprintf("%+v", indexes[:40]))
}

func (e *Editor) Draw(screen tcell.Screen) {
	e.screen = screen
	e.Box.DrawForSubclass(screen, e)

	x, y, w, h := e.Box.GetInnerRect()

	// print mode
	if e.oneLineMode {
		tview.Print(screen, "("+e.mode.ShortString()+") ", x, y, 4, tview.AlignLeft, tcell.ColorYellow)
		x += 4
		w -= 4
	} else if e.searchEditor != nil {
		defer e.searchEditor.Draw(screen)
	} else {
		modeColor := tcell.ColorLightGray
		// modeBg := tcell.ColorWhite
		if e.mode == insert {
			modeColor = tcell.ColorGreen
			// modeBg = tcell.ColorLightGreen
		} else if e.mode == replace {
			modeColor = tcell.ColorPink
			// modeBg = tcell.ColorPurple
		}
		_, modeWidth := tview.Print(screen, e.mode.String(), x, y+h-1, w, tview.AlignLeft, modeColor)
		_, modeTxtWidth := tview.Print(screen, " mode", x+modeWidth, y+h-1, w-modeWidth, tview.AlignLeft, tcell.ColorWhite)
		if len(e.pending) > 0 {
			tview.Print(screen, "("+strings.Join(e.pending, "")+")", x+modeWidth+modeTxtWidth+1, y+h-1, w-(x+modeWidth+modeTxtWidth), tview.AlignLeft, tcell.ColorYellow)
		}
		h--
	}

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
	if !e.oneLineMode {
		lineNumberDigit := len(strconv.Itoa(len(e.spansPerLines)))
		lineNumberWidth := lineNumberDigit + 1
		textY := y
		lastLine := e.offsets[0] + h
		if lastLine > len(e.spansPerLines) {
			lastLine = len(e.spansPerLines)
		}
		for i, _ := range e.spansPerLines[e.offsets[0]:lastLine] {
			lineNumber := i + e.offsets[0] + 1
			lineNumberText := fmt.Sprintf("%*d", lineNumberDigit, lineNumber)
			tview.Print(screen, lineNumberText, x, textY, lineNumberWidth, tview.AlignLeft, tcell.ColorSlateGray)
			textY++
		}
		x += lineNumberWidth
		w -= lineNumberWidth
	}

	// cursor is after column offset
	if cursorX > e.offsets[1]+w {
		e.offsets[1] = cursorX - w + 1
	}

	textX := x
	textY := y
	lastLine := e.offsets[0] + h
	if lastLine > len(e.spansPerLines) {
		lastLine = len(e.spansPerLines)
	}
	for row, spans := range e.spansPerLines[e.offsets[0]:lastLine] {
		for col, span := range spans {
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

			d, hasDecoration := e.getDecoration(row, col)

			if runes[0] != '\t' {
				bg := tview.Styles.PrimitiveBackgroundColor
				fg := tview.Styles.PrimaryTextColor
				if hasDecoration && d.text == "" {
					fg, bg, _ = d.style.Decompose()
				}

				screen.SetContent(
					textX-e.offsets[1],
					textY,
					runes[0],
					runes[1:],
					tcell.StyleDefault.Foreground(fg).Background(bg),
				)
			}
			textX += width
		}
		textY++
		textX = x
	}

	if e.searchEditor == nil {
		cursorStyle := tcell.CursorStyleSteadyBlock
		if e.mode == insert {
			cursorStyle = tcell.CursorStyleSteadyBar
		} else if e.mode == replace {
			cursorStyle = tcell.CursorStyleSteadyUnderline
		}
		screen.SetCursorStyle(cursorStyle)
		screen.ShowCursor(cursorX+x-e.offsets[1], e.cursor[0]+y-e.offsets[0])
	}
}

func (e *Editor) getDecoration(row, col int) (decoration, bool) {
	for _, d := range e.decorators {
		dec, b := d(row, col)
		if b {
			return dec, true
		}
	}
	return decoration{}, false
}

func (e *Editor) Focus(delegate func(p tview.Primitive)) {
	if e.searchEditor != nil {
		delegate(e.searchEditor)
		return
	}
	e.Box.Focus(delegate)
}

func (e *Editor) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return e.Box.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if e.searchEditor != nil {
			e.searchEditor.InputHandler()(event, setFocus)
			return
		}

		eventName := event.Name()
		if event.Key() == tcell.KeyRune {
			eventName = string(event.Rune())
		} else {
			eventName = strings.ToLower(eventName)
		}
		e.pending = append(e.pending, eventName)

		group := e.mode.ShortString()
		if e.oneLineMode {
			group = "o" + e.mode.ShortString()
		}

		action, anyStartWith := e.keymapper.Get(e.pending, group)
		if action != "" && e.actionRunner[ActionFromString(action)] != nil {
			e.actionRunner[ActionFromString(action)]()
			e.pending = nil
			return
		}
		if anyStartWith {
			return
		}
		e.pending = nil

		// default actions
		switch e.mode {
		case replace:
			switch key := event.Key(); key {
			case tcell.KeyEsc:
				e.ChangeMode(normal)
			case tcell.KeyRune:
				text := string(event.Rune())
				from := e.cursor
				until := [2]int{e.cursor[0], e.cursor[1] + 1}
				e.ReplaceText(text, from, until)
				e.mode = normal
			}

		case insert:
			switch key := event.Key(); key {
			case tcell.KeyEsc:
				e.mode = normal
				if e.cursor[1] == len(e.spansPerLines[e.cursor[0]])-1 {
					e.MoveCursorLeft()
				}
			case tcell.KeyRune:
				text := string(event.Rune())
				e.ReplaceText(text, e.cursor, e.cursor)
				e.MoveCursorRight()
				e.SaveChanges()
				e.undoOffset--
			case tcell.KeyEnter:
				if e.oneLineMode && e.onDoneFunc != nil {
					e.onDoneFunc(e.text)
					return
				}
				e.ReplaceText("\n", e.cursor, e.cursor)
				e.MoveCursorDown()
				e.cursor[1] = 0
				e.SaveChanges()
				e.undoOffset--
			case tcell.KeyTab:
				e.ReplaceText("\t", e.cursor, e.cursor)
				e.MoveCursorRight()
				e.SaveChanges()
				e.undoOffset--
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
				e.SaveChanges()
				e.undoOffset--
			}
		}
	})
}

// n must be more than or equal to 1
func (e *Editor) GetNextMotion(m string, n int) [2]int {
	if e.motionIndexes[m] == nil {
		return e.cursor
	}
	if len(e.motionIndexes[m]) == 1 {
		return [2]int{e.motionIndexes[m][0][0], e.motionIndexes[m][0][1]}
	}
	if n < 1 {
		n = 1
	}
	n--

	row := e.cursor[0]
	col := e.cursor[1]
	for i, index := range e.motionIndexes[m] {
		if index[0] < row {
			continue
		}

		if index[0] > row {
			col = -1
		}

		if index[1] > col {
			idx := (i + n) % len(e.motionIndexes[m])
			return [2]int{e.motionIndexes[m][idx][0], e.motionIndexes[m][idx][1]}
		}
	}

	if strings.ToLower(m) != "n" {
		return e.cursor
	}
	idx := (0 + n) % len(e.motionIndexes[m])
	return [2]int{e.motionIndexes[m][idx][0], e.motionIndexes[m][idx][1]}
}

// n must be greater or equal to 1
func (e *Editor) GetPrevMotion(m string, n int) [2]int {
	if e.motionIndexes[m] == nil {
		return e.cursor
	}
	if len(e.motionIndexes[m]) == 1 {
		return [2]int{e.motionIndexes[m][0][0], e.motionIndexes[m][0][1]}
	}
	if n < 1 {
		n = 1
	}
	n--

	row := e.cursor[0]
	col := e.cursor[1]
	widestLine := 0
	for _, spans := range e.spansPerLines {
		if len(spans) > widestLine {
			widestLine = len(spans)
		}
	}

	for i, _ := range e.motionIndexes[m] {
		i = len(e.motionIndexes[m]) - 1 - i
		index := e.motionIndexes[m][i]

		if index[0] > row {
			continue
		}

		if index[0] < row {
			col = widestLine
		}

		if index[1] < col {
			idx := (i - n) % len(e.motionIndexes[m])
			if idx < 0 {
				idx += len(e.motionIndexes[m])
			}
			return [2]int{e.motionIndexes[m][idx][0], e.motionIndexes[m][idx][1]}
		}
	}

	if strings.ToLower(m) != "n" {
		return e.cursor
	}
	idx := (len(e.motionIndexes[m]) - 1 - n) % len(e.motionIndexes[m])
	if idx < 0 {
		idx += len(e.motionIndexes[m])
	}
	return [2]int{e.motionIndexes[m][idx][0], e.motionIndexes[m][idx][1]}
}

func (e *Editor) MoveCursorRight() {
	blockOffset := 1
	if e.mode == insert {
		blockOffset = 0
	}
	if e.cursor[1]+blockOffset >= len(e.spansPerLines[e.cursor[0]])-1 {
		return
	}

	e.cursor[1]++
}

func (e *Editor) MoveCursorEndOfLine() {
	if e.cursor[1] >= len(e.spansPerLines[e.cursor[0]])-1 {
		return
	}

	blockOffset := 0
	if e.mode == insert {
		blockOffset = 1
	}

	e.cursor[1] = len(e.spansPerLines[e.cursor[0]]) - 2 + blockOffset
}

func (e *Editor) MoveCursorLeft() {
	if e.cursor[1] < 1 {
		return
	}

	e.cursor[1]--
}

func (e *Editor) MoveCursorStartOfLine() {
	if e.cursor[1] < 1 {
		return
	}

	e.cursor[1] = 0
}

func (e *Editor) MoveCursorDown() {
	if e.cursor[0] >= len(e.spansPerLines)-1 {
		return
	}

	currentRowWidth := 0
	for _, span := range e.spansPerLines[e.cursor[0]][:e.cursor[1]] {
		currentRowWidth += span.width
	}

	blockOffset := 0
	if e.mode == insert {
		blockOffset = 1
	}
	belowRowX := 0
	belowRowWidth := 0
	belowRowSpans := e.spansPerLines[e.cursor[0]+1]
	maxOffset := len(belowRowSpans) - 2 + blockOffset
	if maxOffset < 0 {
		maxOffset = 0
	}
	for _, span := range belowRowSpans[:maxOffset] {
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

func (e *Editor) MoveCursorHalfPageDown() {
	_, _, _, h := e.Box.GetInnerRect()
	h-- // exclude status line

	if e.cursor[0] >= len(e.spansPerLines)-1 {
		return
	}

	currentRowWidth := 0
	for _, span := range e.spansPerLines[e.cursor[0]][:e.cursor[1]] {
		currentRowWidth += span.width
	}

	blockOffset := 0
	if e.mode == insert {
		blockOffset = 1
	}
	halfPageDownRowX := 0
	halfPageDownRowWidth := 0
	halfPageDownIdx := e.cursor[0] + h/2
	if halfPageDownIdx > len(e.spansPerLines)-1 {
		halfPageDownIdx = len(e.spansPerLines) - 1
	}
	halfPageDownRowSpans := e.spansPerLines[halfPageDownIdx]
	maxOffset := len(halfPageDownRowSpans) - 2 + blockOffset
	if maxOffset < 0 {
		maxOffset = 0
	}
	for _, span := range halfPageDownRowSpans[:maxOffset] {
		if span.runes == nil {
			break
		}
		if halfPageDownRowWidth+span.width > currentRowWidth {
			break
		}
		halfPageDownRowX++
		halfPageDownRowWidth += span.width
	}

	distanceFromTop := e.cursor[0] - e.offsets[0]
	e.cursor[0] = halfPageDownIdx
	e.cursor[1] = halfPageDownRowX

	newRowOffset := e.cursor[0] - distanceFromTop
	if newRowOffset > len(e.spansPerLines)-h {
		newRowOffset = len(e.spansPerLines) - h
	} else if newRowOffset < 0 {
		newRowOffset = 0
	}
	e.offsets[0] = newRowOffset
}

func (e *Editor) MoveCursorLastLine() {
	if e.cursor[0] >= len(e.spansPerLines)-2 {
		return
	}

	currentRowWidth := 0
	for _, span := range e.spansPerLines[e.cursor[0]][:e.cursor[1]] {
		currentRowWidth += span.width
	}

	lastRowX := 0
	lastRowWidth := 0
	for _, span := range e.spansPerLines[len(e.spansPerLines)-1] {
		if span.runes == nil {
			break
		}
		if lastRowWidth+span.width > currentRowWidth {
			break
		}
		lastRowX++
		lastRowWidth += span.width
	}

	e.cursor[0] = len(e.spansPerLines) - 1
	e.cursor[1] = lastRowX
}

func (e *Editor) MoveCursorUp() {
	if e.cursor[0] < 1 {
		return
	}

	currentRowWidth := 0
	for _, span := range e.spansPerLines[e.cursor[0]][:e.cursor[1]] {
		currentRowWidth += span.width
	}

	blockOffset := 0
	if e.mode == insert {
		blockOffset = 1
	}
	aboveRowX := 0
	aboveRowWidth := 0
	aboveRowSpans := e.spansPerLines[e.cursor[0]-1]
	maxOffset := len(aboveRowSpans) - 2 + blockOffset
	if maxOffset < 0 {
		maxOffset = 0
	}
	for _, span := range aboveRowSpans[:maxOffset] {
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

func (e *Editor) MoveCursorHalfPageUp() {
	_, _, _, h := e.Box.GetInnerRect()
	h-- // exclude status line

	if e.cursor[0] < 1 {
		return
	}

	currentRowWidth := 0
	for _, span := range e.spansPerLines[e.cursor[0]][:e.cursor[1]] {
		currentRowWidth += span.width
	}

	blockOffset := 0
	if e.mode == insert {
		blockOffset = 1
	}
	halfPageUpRowX := 0
	halfPageUpRowWidth := 0
	halfPageUpIdx := e.cursor[0] - h/2
	if halfPageUpIdx < 0 {
		halfPageUpIdx = 0
	}
	halfPageUpRowSpans := e.spansPerLines[halfPageUpIdx]
	maxOffset := len(halfPageUpRowSpans) - 2 + blockOffset
	if maxOffset < 0 {
		maxOffset = 0
	}
	for _, span := range halfPageUpRowSpans[:maxOffset] {
		if span.runes == nil {
			break
		}
		if halfPageUpRowWidth+span.width > currentRowWidth {
			break
		}
		halfPageUpRowX++
		halfPageUpRowWidth += span.width
	}

	distanceFromTop := e.cursor[0] - e.offsets[0]
	e.cursor[0] = halfPageUpIdx
	e.cursor[1] = halfPageUpRowX

	newRowOffset := e.cursor[0] - distanceFromTop
	if newRowOffset > len(e.spansPerLines)-h {
		newRowOffset = len(e.spansPerLines) - h
	} else if newRowOffset < 0 {
		newRowOffset = 0
	}
	e.offsets[0] = newRowOffset
}

func (e *Editor) MoveCursorFirstLine() {
	if e.cursor[0] <= 1 {
		return
	}

	currentRowWidth := 0
	for _, span := range e.spansPerLines[e.cursor[0]][:e.cursor[1]] {
		currentRowWidth += span.width
	}

	firstRowX := 0
	firstRowWidth := 0
	for _, span := range e.spansPerLines[0] {
		if span.runes == nil {
			break
		}
		if firstRowWidth+span.width > currentRowWidth {
			break
		}
		firstRowX++
		firstRowWidth += span.width
	}

	e.cursor[0] = 0
	e.cursor[1] = firstRowX
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

	e.SaveChanges()
	e.SetText(b.String(), e.cursor)
}

func (e *Editor) SaveChanges() {
	maxUndoOffset := e.undoOffset + 1
	if maxUndoOffset > len(e.undoStack) {
		maxUndoOffset = len(e.undoStack)
	}
	e.undoStack = e.undoStack[:maxUndoOffset]
	e.undoStack = append(e.undoStack, undoStackItem{
		text:   e.text,
		cursor: [2]int{e.cursor[0], e.cursor[1]},
	})
	e.undoOffset = maxUndoOffset
}

func (e *Editor) Done() {
	if e.onDoneFunc == nil {
		return
	}

	e.onDoneFunc(e.text)
}

func (e *Editor) Exit() {
	if e.onExitFunc == nil {
		return
	}

	e.onExitFunc()
}

func (e *Editor) Redo() {
	if len(e.undoStack) < 1 {
		return
	}
	if e.undoOffset+1 >= len(e.undoStack)-1 {
		return
	}
	redo := e.undoStack[e.undoOffset+2]
	e.undoOffset++
	e.SetText(redo.text, redo.cursor)
}

func (e *Editor) EnableSearch() {
	x, y, w, h := e.Box.GetInnerRect()
	se := New(e.keymapper).SetOneLineMode(true)
	se.SetText("", [2]int{0, 0})
	se.SetRect(x, y+h-1, w, 1)
	se.mode = insert
	se.onDoneFunc = func(s string) {
		e.searchEditor = nil

		foundMatches := e.buildSearchIndexes(s)
		if foundMatches {
			e.MoveMotion("n", 1)
		}
	}
	se.onExitFunc = func() {
		e.searchEditor = nil
	}
	e.searchEditor = se
}

func (e *Editor) ChangeMode(m mode) {
	e.mode = m
}

func (e *Editor) DeleteUnderCursor() {
	from := e.cursor
	until := [2]int{e.cursor[0], e.cursor[1] + 1}
	e.ReplaceText("", from, until)
	e.cursor = from
}

func (e *Editor) Undo() {
	if len(e.undoStack) < 1 {
		return
	}
	if e.undoOffset < 0 {
		return
	}
	undo := e.undoStack[e.undoOffset]
	e.undoOffset--
	e.SetText(undo.text, undo.cursor)
}

func (e *Editor) InsertBelow() {
	e.MoveCursorEndOfLine()
	e.cursor[1]++
	e.ReplaceText("\n", e.cursor, e.cursor)
	e.MoveCursorDown()
	e.cursor[1] = 0
	e.SaveChanges()
	e.undoOffset--
	e.mode = insert
}

func (e *Editor) InsertAbove() {
	e.MoveCursorStartOfLine()
	e.ReplaceText("\n", e.cursor, e.cursor)
	e.cursor[1] = 0
	e.SaveChanges()
	e.undoOffset--
	e.mode = insert
}

func (e *Editor) ChangeUntilEndOfLine() {
	from := e.cursor
	until := [2]int{e.cursor[0], len(e.spansPerLines[e.cursor[0]]) - 1}
	e.ReplaceText("", from, until)
	e.SaveChanges()
	e.undoOffset--
	e.mode = insert
}

func (e *Editor) DeleteUntilEndOfLine() {
	if len(e.spansPerLines[e.cursor[0]]) <= 1 {
		return
	}
	from := e.cursor
	until := [2]int{e.cursor[0], len(e.spansPerLines[e.cursor[0]]) - 1}
	e.ReplaceText("", from, until)
	e.cursor[1]--
	if e.cursor[1] < 0 {
		e.cursor[1] = 0
	}
	e.SaveChanges()
	e.undoOffset--
}

func (e *Editor) DeleteLine() {
	if len(e.spansPerLines) <= 1 {
		return
	}
	from := [2]int{e.cursor[0], 0}
	until := [2]int{e.cursor[0] + 1, 0}
	if e.cursor[0] == len(e.spansPerLines)-1 {
		aboveRow := e.cursor[0] - 1
		from = [2]int{aboveRow, len(e.spansPerLines[aboveRow]) - 1}
		until = [2]int{e.cursor[0], len(e.spansPerLines[e.cursor[0]]) - 1}
	}
	e.ReplaceText("", from, until)
	e.cursor[0]--
	if e.cursor[0] < 0 {
		e.cursor[0] = 0
	}
	e.SaveChanges()
	e.undoOffset--
}

func (e *Editor) InsertAfter() {
	e.mode = insert
	e.MoveCursorRight()
}

func (e *Editor) InsertEndOfLine() {
	e.mode = insert
	e.MoveCursorEndOfLine()
}

func (e *Editor) MoveCursorFirstNonWhitespace() {
	rg := regexp.MustCompile(`\S`)
	idx := rg.FindStringIndex(strings.Split(e.text, "\n")[e.cursor[0]])
	if len(idx) == 0 {
		e.cursor[1] = 0
		return
	}

	e.cursor[1] = idx[0]
}

func (e *Editor) MoveMotion(motion string, n int) {
	if n < 0 {
		e.cursor = e.GetPrevMotion(motion, n*-1)
		return
	}
	e.cursor = e.GetNextMotion(motion, n)
}
