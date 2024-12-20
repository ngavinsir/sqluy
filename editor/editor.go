package editor

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/ngavinsir/sqluy/clipboard"
	"github.com/ngavinsir/sqluy/vim"
	"github.com/ngavinsir/treesittergo"
	"github.com/rivo/tview"
	"github.com/rivo/uniseg"
)

type (
	keymapper interface {
		Get(keys []string, group string) ([]string, bool)
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

	decorator func(x, y, width, height int)

	Editor struct {
		mutex             sync.Mutex
		keymapper         keymapper
		viewModalFunc     func(string)
		onDoneFunc        func(*Editor, string)
		onTextChangedFunc func(string)
		delayDrawFunc     func(time.Time, func())
		onExitFunc        func()
		*tview.Box
		searchEditor        *Editor
		actionRunner        map[Action]func()
		operatorRunner      map[Action]func(target [2]int)
		motionRunner        map[Action]func() [2]int
		runeRunner          map[Action]func(r rune)
		motionIndexes       map[rune][][3]int
		flashIndexes        map[rune][2]int
		reverseFlashIndexes map[[2]int]rune
		motionIndexesMutex  *sync.RWMutex
		decorations         map[[2]int]decoration
		highlightIndexes    map[[2]int]string
		text                string
		spansPerLines       [][]span
		pending             []string
		undoStack           []undoStackItem
		decorators          []decorator
		cursor              [2]int
		disabled            bool
		visualStart         [2]int
		offsets             [2]int
		pendingCount        int
		tabSize             int
		editCount           atomic.Uint64
		undoOffset          int
		pendingAction       Action
		lastMotion          Action
		mode                mode
		oneLineMode         bool
		waitingForMotion    bool
		yankOnVisual        bool // for yank indicator utilizng ModeVisual mode

		parser  treesittergo.Parser
		ts      treesittergo.Treesitter
		sqlLang treesittergo.Language
	}
)

var (
	//go:embed sql.highlights.scm
	sqlHighlightsQuery string

	flashAlphabet = "abcdefghijkmnpqrtwxyzABCDEFGHJKLMNPQRTUVWXY"

	matchBlocks              = []rune{'{', '}', '[', ']', '(', ')', '"', '\'', '`'}
	directionlessMatchBlocks = []rune{'"', '`', '\''}
	matchBlockDirection      = map[rune]int{
		'{': 1,
		'}': -1,
		'[': 1,
		']': -1,
		'(': 1,
		')': -1,
	}
	matchingBlock = map[rune]rune{
		'{':  '}',
		'}':  '{',
		'[':  ']',
		']':  '[',
		'(':  ')',
		')':  '(',
		'"':  '"',
		'\'': '\'',
		'`':  '`',
	}

	colorMap = map[string]tcell.Style{
		"variable":              tcell.StyleDefault.Foreground(tcell.NewHexColor(0xc0caf5)),
		"function.call":         tcell.StyleDefault.Foreground(tcell.NewHexColor(0x7aa2f7)),
		"keyword.operator":      tcell.StyleDefault.Foreground(tcell.NewHexColor(0x89ddff)),
		"keyword":               tcell.StyleDefault.Foreground(tcell.NewHexColor(0x9d7cd8)),
		"type":                  tcell.StyleDefault.Foreground(tcell.NewHexColor(0x2ac3de)),
		"variable.member":       tcell.StyleDefault.Foreground(tcell.NewHexColor(0x73daca)),
		"type.builtin":          tcell.StyleDefault.Foreground(tcell.NewHexColor(0x2ac3de)),
		"string":                tcell.StyleDefault.Foreground(tcell.NewHexColor(0x9ece6a)),
		"operator":              tcell.StyleDefault.Foreground(tcell.NewHexColor(0x89ddff)),
		"keyword.modifier":      tcell.StyleDefault.Foreground(tcell.NewHexColor(0x9d7cd8)),
		"punctuation.bracket":   tcell.StyleDefault.Foreground(tcell.NewHexColor(0xa9b1d6)),
		"punctuation.delimiter": tcell.StyleDefault.Foreground(tcell.NewHexColor(0x89ddff)),
		"error":                 tcell.StyleDefault.Underline(tcell.UnderlineStyleCurly, tcell.ColorRed),
	}

	rgFirstNonWhitespace = regexp.MustCompile(`\S`)
	rgMotioneOne         = regexp.MustCompile(`([^a-zA-Z0-9_À-ÿ\s])(?:[a-zA-Z0-9_À-ÿ\s]|$)`)
	rgMotioneTwo         = regexp.MustCompile(`([a-zA-Z0-9_À-ÿ])(?:[^a-zA-Z0-9_À-ÿ]|$)`)
	rgMotionwOne         = regexp.MustCompile(`(?:^|[^a-zA-Z0-9_À-ÿ])([a-zA-Z0-9_À-ÿ])`)
	rgMotionwTwo         = regexp.MustCompile(`(?:^|[a-zA-Z0-9_À-ÿ\s])([^a-zA-Z0-9_À-ÿ\s])`)
	rgMotionW            = regexp.MustCompile(`\s(\S)`)
	rgMotionE            = regexp.MustCompile(`\S(?:[^\S\n]|$)`)
)

func New(options ...func(*Editor)) *Editor {
	ts, err := treesittergo.New(context.Background())
	if err != nil {
		panic(err)
	}
	parser, err := ts.NewParser(context.Background())
	if err != nil {
		panic(err)
	}
	sqlLang, err := ts.LanguageSQL(context.Background())
	if err != nil {
		panic(err)
	}
	parser.SetLanguage(context.Background(), sqlLang)

	e := &Editor{
		tabSize:          4,
		Box:              tview.NewBox().SetBorder(true).SetTitle("Editor").SetTitleAlign(tview.AlignLeft),
		decorations:      make(map[[2]int]decoration),
		highlightIndexes: make(map[[2]int]string),
		ts:               ts,
		parser:           parser,
		sqlLang:          sqlLang,
	}
	for _, option := range options {
		option(e)
	}
	// e.SetText("amsok", [2]int{0, 0})
	e.SetText(`WITH RECURSIVE trip AS (
  SELECT c.city_id AS start_city,
    ARRAY[c.city_id] AS route,
    cast(c.name AS varchar(100)) AS route_text,
    c.city_id AS leg_start_city,
    c.city_id AS leg_end_city,
    0 AS trip_count,
    0 AS leg_length,
    0 AS total_length,
  FROM cities c
UNION ALL
  SELECT
    trip.start_city,
    trip.route || t.destination_city_id,
    cast(trip.route_text || ',' || c.name AS varchar(100)),
    t.departure_city_id,
    t.destination_city_id,
    trip.trip_count + 1,
    d.distance,
    trip.total_length + d.distance
  FROM trains t
  INNER JOIN trip
    ON t.departure_city_id =  trip.leg_end_city
  INNER JOIN citypairs cps
    ON t.departure_city_id = cps.city_id
  INNER JOIN citypairs cpe
    ON t.destination_city_id = cpe.city_id AND
       cpe.citypair_id = cps.citypair_id
  INNER JOIN distances d
    ON cps.citypair_id = d.citypair_id
  INNER JOIN cities c
     ON t.destination_city_id = c.city_id
  WHERE NOT (array[t.destination_city_id] <@ trip.route))
SELECT *
FROM trip
WHERE trip_count = 2
AND start_city = (SELECT city_id FROM cities WHERE name = 'Edinburgh');`, [2]int{3, 8})

	e.onExitFunc = func() {
		e.ChangeMode(ModeNormal)
		e.ResetMotionIndexes()
	}

	e.actionRunner = map[Action]func(){
		ActionDone: e.Done,
		ActionExit: e.Exit,
		ActionInsert: func() {
			e.ChangeMode(ModeInsert)
		},
		ActionRedo:                 e.Redo,
		ActionUndo:                 e.Undo,
		ActionMoveHalfPageDown:     e.MoveCursorHalfPageDown,
		ActionMoveHalfPageUp:       e.MoveCursorHalfPageUp,
		ActionDeleteUnderCursor:    e.DeleteUnderCursor,
		ActionInsertAfter:          e.InsertAfter,
		ActionInsertEndOfLine:      e.InsertEndOfLine,
		ActionInsertBelow:          e.InsertBelow,
		ActionInsertAbove:          e.InsertAbove,
		ActionChangeUntilEndOfLine: e.ChangeUntilEndOfLine,
		ActionDeleteUntilEndOfLine: e.DeleteUntilEndOfLine,
		ActionDeleteLine: func() {
			for range e.getActionCount() {
				e.DeleteLine()
			}
		},
		ActionPasteBefore: func() {
			txt, _ := clipboard.Read()
			if txt == "" {
				return
			}

			hasNewLine := uniseg.HasTrailingLineBreakInString(txt)
			if hasNewLine {
				c := [2]int{e.cursor[0], 0}
				e.ReplaceText(txt, c, c)
			} else {
				c := [2]int{e.cursor[0], e.cursor[1] - 1}
				if c[1] < 0 {
					c[1] = 0
				}
				e.ReplaceText(txt, c, c)
			}
		},
		ActionPasteAfter: func() {
			txt, _ := clipboard.Read()
			if txt == "" {
				return
			}

			hasNewLine := uniseg.HasTrailingLineBreakInString(txt)
			if hasNewLine {
				c := [2]int{e.cursor[0] + 1, 0}
				e.ReplaceText(txt, c, c)
			} else {
				c := [2]int{e.cursor[0], e.cursor[1] + 1}
				e.ReplaceText(txt, c, c)
			}
		},
		ActionVisualLine: func() {
			if e.mode == ModeVLine {
				e.ChangeMode(ModeNormal)
				return
			}
			e.visualStart = [2]int{e.cursor[0], 0}
			e.ChangeMode(ModeVLine)
		},
		ActionMoveMatchBlock: func() {
			e.MoveCursorTo(e.GetMatchingBlock(e.cursor))
		},
		ActionReplace: func() {
			e.ChangeMode(ModeReplace)
		},
		ActionMoveNextSearch: func() {
			e.MoveMotion('n', e.getActionCount())
		},
		ActionMovePrevSearch: func() {
			e.MoveMotion('n', -e.getActionCount())
		},
		ActionSwitchVisualStart: func() {
			if e.mode != ModeVisual {
				return
			}

			e.visualStart, e.cursor = e.cursor, e.visualStart
		},
		ActionMoveNextFind: func() {
			if e.motionIndexes['f'] != nil && !strings.Contains(e.lastMotion.String(), "back") {
				e.MoveMotion('f', e.getActionCount())
			} else if e.motionIndexes['f'] != nil {
				e.MoveMotion('f', -e.getActionCount())
			} else if e.motionIndexes['t'] != nil {
				e.MoveMotion('t', e.getActionCount())
			} else if e.motionIndexes['T'] != nil {
				e.MoveMotion('T', -e.getActionCount())
			}
		},
		ActionMovePrevFind: func() {
			if e.motionIndexes['f'] != nil && !strings.Contains(e.lastMotion.String(), "back") {
				e.MoveMotion('f', -e.getActionCount())
			} else if e.motionIndexes['f'] != nil {
				e.MoveMotion('f', e.getActionCount())
			} else if e.motionIndexes['t'] != nil {
				e.MoveMotion('t', -e.getActionCount())
			} else if e.motionIndexes['T'] != nil {
				e.MoveMotion('T', e.getActionCount())
			}
		},
	}

	e.motionRunner = map[Action]func() [2]int{
		ActionMoveEndOfLine:          e.GetEndOfLineCursor,
		ActionMoveStartOfLine:        e.GetStartOfLineCursor,
		ActionMoveFirstNonWhitespace: e.GetFirstNonWhitespaceCursor,
		ActionMoveDown:               e.GetDownCursor,
		ActionMoveUp:                 e.GetUpCursor,
		ActionMoveLeft:               e.GetLeftCursor,
		ActionMoveRight:              e.GetRightCursor,
		ActionMoveLastLine:           e.GetLastLineCursor,
		ActionMoveFirstLine:          e.GetFirstLineCursor,
		ActionMoveStartOfWord:        e.GetStartOfWordCursor,
		ActionMoveStartOfBigWord:     e.GetStartOfBigWordCursor,
		ActionMoveEndOfBigWord:       e.GetEndOfBigWordCursor,
		ActionMoveBackEndOfBigWord:   e.GetBackEndOfBigWordCursor,
		ActionMoveBackStartOfBigWord: e.GetBackStartOfBigWordCursor,
		ActionMoveEndOfWord:          e.GetEndOfWordCursor,
		ActionMoveBackEndOfWord:      e.GetBackEndOfWordCursor,
		ActionMoveBackStartOfWord:    e.GetBackStartOfWordCursor,
		ActionEnableSearch:           e.EnableSearch,
		ActionFlash:                  e.Flash,
		ActionTil:                    e.GetTilCursor,
		ActionTilBack:                e.GetTilBackCursor,
		ActionFind:                   e.GetFindCursor,
		ActionFindBack:               e.GetFindBackCursor,
		ActionInside:                 e.GetInsideOrAroundCursor,
		ActionAround:                 e.GetInsideOrAroundCursor,
	}

	e.operatorRunner = map[Action]func(target [2]int){
		ActionNone:   e.MoveCursorTo,
		ActionChange: e.ChangeUntil,
		ActionDelete: e.DeleteUntil,
		ActionYank:   e.YankUntil,
		ActionVisual: e.VisualUntil,
	}

	e.runeRunner = map[Action]func(r rune){
		ActionTil:      e.AcceptRuneTil,
		ActionTilBack:  e.AcceptRuneTilBack,
		ActionFind:     e.AcceptRuneFind,
		ActionFindBack: e.AcceptRuneFind,
		ActionInside:   e.AcceptRuneInside,
		ActionAround:   e.AcceptRuneAround,
	}

	e.decorators = []decorator{
		e.highlightDecorator,
		e.searchDecorator,
		e.visualDecorator,
		e.flashDecorator,
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

func (e *Editor) SetDelayDrawFunc(f func(time.Time, func())) *Editor {
	e.delayDrawFunc = f
	return e
}

func (e *Editor) SetText(text string, cursor [2]int) *Editor {
	if e.onTextChangedFunc != nil {
		e.onTextChangedFunc(text)
	}

	editCount := e.editCount.Add(1)
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

	e.MoveCursorToLine(cursor[0])

	e.motionIndexes = make(map[rune][][3]int)
	e.highlightIndexes = make(map[[2]int]string)
	spansPerLines := append([][]span{}, e.spansPerLines...)
	go e.buildMotionwIndexes(editCount, e.text, spansPerLines)
	go e.buildMotioneIndexes(editCount, e.text, spansPerLines)
	go e.buildMotionWIndexes(editCount, e.text, spansPerLines)
	go e.buildMotionEIndexes(editCount, e.text, spansPerLines)

	if !e.oneLineMode {
		e.buildTreesitter(e.text)
	}

	return e
}

func (e *Editor) buildTreesitter(text string) {
	tree, err := e.parser.ParseString(context.Background(), text)
	if err != nil {
		panic(err)
	}

	q, err := e.ts.NewQuery(context.Background(), sqlHighlightsQuery, e.sqlLang)
	if err != nil {
		panic(err)
	}
	qc, err := e.ts.NewQueryCursor(context.Background())
	if err != nil {
		panic(err)
	}
	rootNode, err := tree.RootNode(context.Background())
	if err != nil {
		panic(err)
	}
	qc.Exec(context.Background(), q, rootNode)
	lastEnd := uint64(0)
	// Iterate over query results
	for {
		m, ok, err := qc.NextMatch(context.Background())
		if err != nil {
			panic(err)
		}
		if !ok {
			break
		}
		if m.Captures == nil {
			continue
		}
		for _, c := range m.Captures {
			nodeStartByte, err := c.Node.StartByte(context.Background())
			if err != nil {
				panic(err)
			}
			if nodeStartByte < lastEnd {
				continue
			}
			captureName, err := q.CaptureNameForID(context.Background(), c.ID)
			if err != nil {
				panic(err)
			}
			nodeEndByte, err := c.Node.EndByte(context.Background())
			if err != nil {
				panic(err)
			}
			lastEnd = nodeEndByte
			e.highlightIndexes[[2]int{int(nodeStartByte), int(nodeEndByte)}] = captureName
		}
	}

	i := e.ts.NewIterator(rootNode, treesittergo.DFSMode)
	i.ForEach(context.Background(), func(n treesittergo.Node) error {
		nodeIsError, err := n.IsError(context.Background())
		if err != nil {
			panic(err)
		}
		if nodeIsError {
			nodeStartByte, err := n.StartByte(context.Background())
			if err != nil {
				panic(err)
			}
			nodeEndByte, err := n.EndByte(context.Background())
			if err != nil {
				panic(err)
			}
			e.highlightIndexes[[2]int{int(nodeStartByte), int(nodeEndByte)}] = "error"
		}
		return nil
	})
}

func (e *Editor) buildSearchIndexes(group rune, query string, offset, y, maxY int) bool {
	if offset < 0 {
		query = "([^" + query + "])" + query
	} else if offset > 0 {
		query += "([^" + query + "])"
	}

	foundMatches := false
	rg := regexp.MustCompile(query)

	var indexes [][3]int
	textPerLines := strings.Split(e.text, "\n")
	if maxY <= 0 || maxY > len(textPerLines) {
		maxY = len(textPerLines)
	}
	for i, line := range textPerLines[y:maxY] {
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
			if offset != 0 && len(m) < 4 {
				continue
			}
			if offset == 0 && len(m) < 1 {
				continue
			}

			foundMatches = true
			if offset != 0 {
				indexes = append(indexes, [3]int{i, mapper[m[2]], mapper[m[2]]})
			} else {
				indexes = append(indexes, [3]int{i, mapper[m[0]], mapper[m[1]-1]})
			}
		}
	}

	e.motionIndexes[group] = indexes
	return foundMatches
}

func (e *Editor) buildMotionwIndexes(editCount uint64, text string, spansPerLines [][]span) {
	var indexes [][3]int
	for i, line := range strings.Split(text, "\n") {
		if e.editCount.Load() > editCount {
			return
		}
		if len(line) == 0 {
			continue
		}

		bytesWidthSum := 0
		for _, s := range spansPerLines[i] {
			bytesWidthSum += s.bytesWidth
		}
		mapper := make([]int, bytesWidthSum)
		mapperIdx := 0
		for i, s := range spansPerLines[i] {
			for j := range s.bytesWidth {
				mapper[mapperIdx+j] = i
			}
			mapperIdx += s.bytesWidth
		}

		matchesOne := rgMotionwOne.FindAllStringSubmatchIndex(line, -1)
		matchesTwo := rgMotionwTwo.FindAllStringSubmatchIndex(line, -1)

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

	if e.editCount.Load() > editCount {
		return
	}

	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.motionIndexes['w'] = indexes
}

func (e *Editor) buildMotionEIndexes(editCount uint64, text string, spansPerLines [][]span) {
	var indexes [][3]int
	for i, line := range strings.Split(text, "\n") {
		if e.editCount.Load() > editCount {
			return
		}
		if len(line) == 0 {
			continue
		}

		bytesWidthSum := 0
		for _, s := range spansPerLines[i] {
			bytesWidthSum += s.bytesWidth
		}
		mapper := make([]int, bytesWidthSum)
		mapperIdx := 0
		for i, s := range spansPerLines[i] {
			for j := range s.bytesWidth {
				mapper[mapperIdx+j] = i
			}
			mapperIdx += s.bytesWidth
		}

		matches := rgMotionE.FindAllStringSubmatchIndex(line, -1)

		for _, m := range matches {
			if len(m) < 2 || m[0] >= m[1] {
				continue
			}

			indexes = append(indexes, [3]int{i, mapper[m[0]], mapper[m[1]-1]})
		}
	}
	if e.editCount.Load() > editCount {
		return
	}

	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.motionIndexes['E'] = indexes
}

func (e *Editor) buildMotionWIndexes(editCount uint64, text string, spansPerLines [][]span) {
	var indexes [][3]int
	for i, line := range strings.Split(text, "\n") {
		if e.editCount.Load() > editCount {
			return
		}
		if len(line) == 0 {
			continue
		}

		bytesWidthSum := 0
		for _, s := range spansPerLines[i] {
			bytesWidthSum += s.bytesWidth
		}
		mapper := make([]int, bytesWidthSum)
		mapperIdx := 0
		for i, s := range spansPerLines[i] {
			for j := range s.bytesWidth {
				mapper[mapperIdx+j] = i
			}
			mapperIdx += s.bytesWidth
		}

		matches := rgMotionW.FindAllStringSubmatchIndex(line, -1)

		for _, m := range matches {
			if len(m) < 4 || m[2] >= m[3] {
				continue
			}

			indexes = append(indexes, [3]int{i, mapper[m[2]], mapper[m[3]-1]})
		}
	}
	if e.editCount.Load() > editCount {
		return
	}

	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.motionIndexes['W'] = indexes
}

func (e *Editor) buildMotioneIndexes(editCount uint64, text string, spansPerLines [][]span) {
	var indexes [][3]int
	for i, line := range strings.Split(text, "\n") {
		if e.editCount.Load() > editCount {
			return
		}
		if len(line) == 0 {
			continue
		}

		bytesWidthSum := 0
		for _, s := range spansPerLines[i] {
			bytesWidthSum += s.bytesWidth
		}
		mapper := make([]int, bytesWidthSum)
		mapperIdx := 0
		for i, s := range spansPerLines[i] {
			for j := range s.bytesWidth {
				mapper[mapperIdx+j] = i
			}
			mapperIdx += s.bytesWidth
		}

		matchesOne := rgMotioneOne.FindAllStringSubmatchIndex(line, -1)
		matchesTwo := rgMotioneTwo.FindAllStringSubmatchIndex(line, -1)

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

	if e.editCount.Load() > editCount {
		return
	}

	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.motionIndexes['e'] = indexes
}

func (e *Editor) Draw(screen tcell.Screen) {
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
		if e.mode == ModeInsert {
			modeColor = tcell.ColorGreen
			// modeBg = tcell.ColorLightGreen
		} else if e.mode == ModeReplace {
			modeColor = tcell.ColorPink
			// modeBg = tcell.ColorPurple
		}
		_, modeWidth := tview.Print(screen, e.mode.String(), x, y+h-1, w, tview.AlignLeft, modeColor)
		_, modeTxtWidth := tview.Print(screen, " mode", x+modeWidth, y+h-1, w-modeWidth, tview.AlignLeft, tcell.ColorWhite)
		pendingWidth := 0
		if len(e.pending) > 0 || e.pendingCount > 0 || e.pendingAction != ActionNone {
			pendingCountTxt := ""
			if e.pendingCount > 0 {
				pendingCountTxt = strconv.Itoa(e.pendingCount)
			}
			_, pendingWidth = tview.Print(screen, "("+pendingCountTxt+strings.Join(e.pending, "")+")", x+modeWidth+modeTxtWidth+1, y+h-1, w-(x+modeWidth+modeTxtWidth), tview.AlignLeft, tcell.ColorYellow)
		}
		posText := fmt.Sprintf("x: %d/%d y: %d/%d", e.cursor[1]+1, len(e.spansPerLines[e.cursor[0]]), e.cursor[0]+1, len(e.spansPerLines))
		tview.Print(screen, posText, x+modeWidth+modeTxtWidth+pendingWidth+1, y+h-1, w-(x+modeWidth+modeTxtWidth+pendingWidth+1), tview.AlignRight, tcell.ColorWhite)
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

	lineNumberDigit := len(strconv.Itoa(len(e.spansPerLines)))
	lineNumberWidth := 0
	if !e.oneLineMode {
		lineNumberWidth = lineNumberDigit + 1
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

	clear(e.decorations)
	for _, decorator := range e.decorators {
		decorator(e.offsets[1], e.offsets[0], w, h)
	}

	for row, spans := range e.spansPerLines[e.offsets[0]:lastLine] {
		row += e.offsets[0]

		// highlight current cursor line
		if e.HasFocus() && !e.oneLineMode && row == e.cursor[0] {
			highlightWidth := w
			if !e.oneLineMode {
				highlightWidth += lineNumberWidth
			}
			for i := range w {
				screen.SetContent(x+i, textY, ' ', nil, tcell.StyleDefault.Background(tcell.ColorGray).Foreground(tcell.ColorWhite))
			}
		}

		// print line numbers
		if !e.oneLineMode {
			lineNumber := row - e.cursor[0]
			if lineNumber < 0 {
				lineNumber *= -1
			}
			if e.cursor[0] == row {
				lineNumber = row + 1
			}
			lineNumberText := fmt.Sprintf("%*d", lineNumberDigit, lineNumber)
			lineNumberColor := tcell.ColorSlateGray
			if e.HasFocus() && row == e.cursor[0] {
				lineNumberColor = tcell.ColorOrange
			}
			tview.Print(screen, lineNumberText, x, textY, lineNumberWidth, tview.AlignLeft, lineNumberColor)
			textX += lineNumberWidth
		}

		for col, span := range spans {
			// draw end of line sentinel decoration if exist, else can break
			if span.runes == nil && col > 0 {
				d, hasDecoration := e.decorations[[2]int{row, col}]
				if !hasDecoration {
					break
				}

				// print bg
				fg, bg, _ := d.style.Decompose()
				if bg == tcell.ColorDefault {
					d.style = d.style.Background(tview.Styles.PrimitiveBackgroundColor)
					if e.HasFocus() && e.cursor[0] == row {
						d.style = d.style.Background(tcell.ColorGray)
					}
				}
				screen.SetContent(
					textX-e.offsets[1],
					textY,
					' ',
					nil,
					d.style,
				)

				// print text
				tview.Print(screen, d.text, textX-e.offsets[1], textY, w-textX-e.offsets[1]+1, tview.AlignLeft, fg)

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
			if textX < x+e.offsets[1] && textX+width > x+e.offsets[1] && runes != nil && runes[0] != '\t' {
				c := textX + width - (x + e.offsets[1])
				runes = []rune(strings.Repeat("<", c))
				textX += width - c
				width = c
			} else if textX+width > x+e.offsets[1]+w && runes != nil && runes[0] != '\t' { // too wide grapheme on the right edge that's not a tab
				c := (x + e.offsets[1] + w) - textX
				runes = []rune(strings.Repeat(">", c))
				width = c
			}

			d, hasDecoration := e.decorations[[2]int{row, col}]
			// print decoration bg
			if hasDecoration {
				_, bg, _ := d.style.Decompose()
				if e.HasFocus() && !e.oneLineMode && row == e.cursor[0] && bg == tcell.ColorDefault {
					bg = tcell.ColorGray
				}
				for i := range span.width {
					screen.SetContent(
						textX-e.offsets[1]+i,
						textY,
						' ',
						nil,
						tcell.StyleDefault.Background(bg),
					)
				}
			}

			// print original text
			if d.text == "" {
				dBg := tcell.ColorDefault
				style := tcell.StyleDefault.Background(tview.Styles.PrimitiveBackgroundColor).Foreground(tview.Styles.PrimaryTextColor)
				if hasDecoration && d.text == "" {
					_, dBg, _ := d.style.Decompose()
					style = d.style
					if dBg == tcell.ColorDefault {
						style = style.Background(tview.Styles.PrimitiveBackgroundColor)
					}
				}
				if e.HasFocus() && !e.oneLineMode && row == e.cursor[0] && dBg == tcell.ColorDefault {
					style = style.Background(tcell.ColorGray)
				}

				if runes != nil && runes[0] != '\t' {
					screen.SetContent(
						textX-e.offsets[1],
						textY,
						runes[0],
						runes[1:],
						style,
					)
				} else {
					for i := range e.tabSize {
						screen.SetContent(
							textX-e.offsets[1]+i,
							textY,
							' ',
							nil,
							style,
						)
					}
				}
			}

			// print decoration text
			if hasDecoration && d.text != "" {
				fg, _, _ := d.style.Decompose()
				tview.Print(screen, d.text, textX-e.offsets[1], textY, span.width, tview.AlignLeft, fg)
			}
			textX += width
		}
		textY++
		textX = x
	}

	// draw cursor
	if e.HasFocus() && e.searchEditor == nil {
		newCursor := [2]int{cursorX + x + lineNumberWidth - e.offsets[1], e.cursor[0] + y - e.offsets[0]}
		cursorStyle := tcell.CursorStyleSteadyBlock
		if e.mode == ModeInsert {
			cursorStyle = tcell.CursorStyleSteadyBar
		} else if e.mode == ModeReplace {
			cursorStyle = tcell.CursorStyleSteadyUnderline
		}
		screen.SetCursorStyle(cursorStyle)
		screen.ShowCursor(newCursor[0], newCursor[1])
		if e.disabled {
			screen.ShowCursor(-1, -1)
		}
	}
}

func (e *Editor) Focus(delegate func(p tview.Primitive)) {
	if e.searchEditor != nil {
		delegate(e.searchEditor)
		return
	}
	e.Box.Focus(delegate)
}

func (e *Editor) SetDisabled(b bool) {
	e.disabled = b
}

func (e *Editor) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return e.Box.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if e.disabled {
			return
		}

		// embedded search editor is not null, send input event to it
		if e.searchEditor != nil {
			e.searchEditor.InputHandler()(event, setFocus)
			return
		}

		// handle unkeymappable actions first, e.g. rune events on insert mode
		switch e.mode {
		case ModeReplace:
			switch key := event.Key(); key {
			case tcell.KeyEsc:
				e.ChangeMode(ModeNormal)
				return
			case tcell.KeyRune:
				text := string(event.Rune())
				from := e.cursor
				until := [2]int{e.cursor[0], e.cursor[1] + 1}
				e.ReplaceText(text, from, until)
				e.mode = ModeNormal
				return
			}

		case ModeInsert:
			switch key := event.Key(); key {
			case tcell.KeyEsc:
				e.mode = ModeNormal
				if e.cursor[1] == len(e.spansPerLines[e.cursor[0]])-1 {
					e.MoveCursorLeft()
				}
				return
			case tcell.KeyRune:
				text := string(event.Rune())
				e.ReplaceText(text, e.cursor, e.cursor)
				e.MoveCursorRight()
				e.SaveChanges()
				e.undoOffset--
				return
			case tcell.KeyEnter:
				if e.oneLineMode && e.onDoneFunc != nil {
					e.onDoneFunc(e, e.text)
					return
				}
				e.ReplaceText("\n", e.cursor, e.cursor)
				e.MoveCursorDown()
				e.cursor[1] = 0
				e.SaveChanges()
				e.undoOffset--
				return
			case tcell.KeyTab:
				e.ReplaceText("\t", e.cursor, e.cursor)
				e.MoveCursorRight()
				e.SaveChanges()
				e.undoOffset--
				return
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
				return
			}
		}

		isDigit := event.Key() == tcell.KeyRune && unicode.IsDigit(event.Rune())

		// append to pending
		eventName := event.Name()
		if event.Key() == tcell.KeyRune {
			eventName = string(event.Rune())
		} else {
			eventName = strings.ToLower(eventName)
		}
		e.pending = append(e.pending, eventName)
		log.Printf("event name: %s\n", eventName)
		log.Printf("event key: %d\n", event.Key())

		// get group
		group := e.mode.ShortString()
		if e.oneLineMode {
			group = "o" + e.mode.ShortString()
		}

		// parse action first try
		actionStrings, anyStartWith := e.keymapper.Get(e.pending, group)
		if actionStrings == nil {
			actionStrings = []string{""}
		}

		for _, actionString := range actionStrings {
			action := ActionFromString(actionString)

			// if not found, try again without pending action in pending for motion only
			if action == ActionNone && e.pendingAction != ActionNone && len(e.pending) > 1 {
				actionStrings, anyStartWith2 := e.keymapper.Get(e.pending[1:], group)
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
			if e.waitingForMotion && event.Key() != tcell.KeyRune {
				e.ResetAction()
				return

				// if waitingForMotion is true and the last motion is waiting for a rune and a rune runner exist for it
			} else if e.waitingForMotion && e.lastMotion.IsWaitingForRune() && e.runeRunner[e.lastMotion] != nil {
				e.runeRunner[e.lastMotion](event.Rune())
				action = e.lastMotion
			}

			// handle operators actions
			// no need to wait for motion action in ModeVisual mode
			if action.IsOperator() && (e.mode == ModeVisual || e.mode == ModeVLine) && action != ActionVisual && action != ActionVisualLine {
				prevMode := e.mode

				if e.mode == ModeVLine {
					if e.cursor[0] > e.visualStart[0] || (e.cursor[0] == e.visualStart[0] && e.cursor[1] > e.visualStart[1]) {
						e.cursor, e.visualStart = e.visualStart, e.cursor
					}
					e.cursor[1] = 0
					e.visualStart[1] = len(e.spansPerLines[e.visualStart[0]]) - 1
				}

				e.operatorRunner[action](e.visualStart)
				if e.mode == prevMode {
					e.mode = ModeNormal
				}
				e.ResetAction()
				return
			}
			// save operator action in pendingAction, wait for the next motion action
			if action.IsOperator() {
				e.pendingAction = action
				return
			}

			// handle motion actions
			// ignore countless motion (e.g. start of line motion) if pending count is not zero
			if action.IsMotion() && (!action.IsCountlessMotion() || e.pendingCount == 0) &&
				e.motionRunner[action] != nil && (action.IsOperatorlessMotion() || e.pendingAction != ActionNone) {
				m := e.motionRunner[action]()
				if vim.IsAsyncMotion(m) {
					e.lastMotion = action
					return
				}

				e.operatorRunner[e.pendingAction](m)
				e.ResetAction()
				return
			}

			// handle the other action
			if e.actionRunner[action] != nil {
				e.actionRunner[action]()
				e.ResetAction()
				return
			}

			// if there's a keymap that starts with runes in pending, don't reset pending
			if anyStartWith {
				return
			}

			// if it's a digit rune event, save it in pending count
			if isDigit {
				e.pendingCount = e.pendingCount*10 + int(event.Rune()-'0')
				e.pending = e.pending[:len(e.pending)-1]
				return
			}
		}

		e.ResetAction()
	})
}

func (e *Editor) getActionCount() int {
	n := 1 + e.pendingCount
	if e.pendingCount > 0 {
		n--
	}
	return n
}

func (e *Editor) MoveCursorTo(to [2]int) {
	e.cursor = to
	e.MoveCursorToLine(e.cursor[0])
}

func (e *Editor) GetNextMotionCursor(m rune, n int, cursor [2]int, inclusive bool) ([2]int, bool) {
	if e.motionIndexes[m] == nil {
		return cursor, false
	}
	if len(e.motionIndexes[m]) == 1 {
		return [2]int{e.motionIndexes[m][0][0], e.motionIndexes[m][0][1]}, true
	}
	if n < 1 {
		n = 1
	}
	n--

	row := cursor[0]
	col := cursor[1]
	if inclusive {
		col--
	}
	for i, index := range e.motionIndexes[m] {
		if index[0] < row {
			continue
		}

		if index[0] > row {
			col = -1
		}

		if index[1] > col {
			idx := (i + n) % len(e.motionIndexes[m])
			return [2]int{e.motionIndexes[m][idx][0], e.motionIndexes[m][idx][1]}, true
		}
	}

	if unicode.ToLower(m) != 'n' {
		return cursor, false
	}
	idx := (0 + n) % len(e.motionIndexes[m])
	return [2]int{e.motionIndexes[m][idx][0], e.motionIndexes[m][idx][1]}, true
}

// n must be greater or equal to 1
func (e *Editor) GetPrevMotionCursor(m rune, n int, cursor [2]int, inclusive bool) ([2]int, bool) {
	if e.motionIndexes[m] == nil {
		return cursor, false
	}
	if len(e.motionIndexes[m]) == 1 {
		return [2]int{e.motionIndexes[m][0][0], e.motionIndexes[m][0][1]}, true
	}
	if n < 1 {
		n = 1
	}
	n--

	row := cursor[0]
	col := cursor[1]
	if inclusive {
		col++
	}
	widestLine := 0
	for _, spans := range e.spansPerLines {
		if len(spans) > widestLine {
			widestLine = len(spans)
		}
	}

	for i := range e.motionIndexes[m] {
		i = len(e.motionIndexes[m]) - 1 - i
		index := e.motionIndexes[m][i]

		if index[0] > row {
			continue
		}

		if index[0] < row {
			col = widestLine
			if inclusive {
				col++
			}
		}

		if index[1] < col {
			idx := (i - n) % len(e.motionIndexes[m])
			if idx < 0 {
				idx += len(e.motionIndexes[m])
			}
			return [2]int{e.motionIndexes[m][idx][0], e.motionIndexes[m][idx][1]}, true
		}
	}

	if unicode.ToLower(m) != 'n' {
		return cursor, false
	}
	idx := (len(e.motionIndexes[m]) - 1 - n) % len(e.motionIndexes[m])
	if idx < 0 {
		idx += len(e.motionIndexes[m])
	}
	return [2]int{e.motionIndexes[m][idx][0], e.motionIndexes[m][idx][1]}, true
}

func (e *Editor) MoveCursorRight() {
	e.MoveCursorTo(e.GetRightCursor())
}

func (e *Editor) GetRightCursor() [2]int {
	n := e.getActionCount()
	x := e.cursor[1] + n
	return [2]int{e.cursor[0], x}
}

func (e *Editor) MoveCursorEndOfLine() {
	e.MoveCursorTo(e.GetEndOfLineCursor())
}

func (e *Editor) GetEndOfLineCursor() [2]int {
	if e.cursor[1] >= len(e.spansPerLines[e.cursor[0]])-1 {
		return e.cursor
	}

	return [2]int{e.cursor[0], len(e.spansPerLines[e.cursor[0]]) - 1}
}

func (e *Editor) MoveCursorLeft() {
	e.cursor = e.GetLeftCursor()
}

func (e *Editor) GetLeftCursor() [2]int {
	if e.cursor[1] < 1 {
		return e.cursor
	}

	n := e.getActionCount()
	x := e.cursor[1] - n
	if x < 0 {
		x = 0
	}
	return [2]int{e.cursor[0], x}
}

func (e *Editor) MoveCursorStartOfLine() {
	e.cursor = e.GetStartOfLineCursor()
}

func (e *Editor) GetStartOfLineCursor() [2]int {
	if e.cursor[1] < 1 {
		return e.cursor
	}

	return [2]int{e.cursor[0], 0}
}

func (e *Editor) MoveCursorDown() {
	e.cursor = e.GetDownCursor()
}

func (e *Editor) GetDownCursor() [2]int {
	n := e.getActionCount() + e.cursor[0]
	return e.GetLineCursor(n)
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
	if e.mode == ModeInsert {
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
	e.cursor = e.GetLastLineCursor()
}

func (e *Editor) GetLastLineCursor() [2]int {
	if e.pendingCount > 0 {
		return e.GetLineCursor(e.pendingCount - 1)
	}
	return e.GetLineCursor(len(e.spansPerLines) - 1)
}

func (e *Editor) GetFirstLineCursor() [2]int {
	if e.pendingCount > 0 {
		return e.GetLineCursor(e.pendingCount - 1)
	}
	return e.GetLineCursor(0)
}

func (e *Editor) MoveCursorUp() {
	e.cursor = e.GetUpCursor()
}

func (e *Editor) GetUpCursor() [2]int {
	n := e.cursor[0] - e.getActionCount()
	return e.GetLineCursor(n)
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
	if e.mode == ModeInsert {
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

func (e *Editor) MoveCursorToLine(n int) {
	e.cursor = e.GetLineCursor(n)
}

func (e *Editor) GetLineCursor(n int) [2]int {
	if n < 0 {
		n = 0
	}
	if n > len(e.spansPerLines)-1 {
		n = len(e.spansPerLines) - 1
	}

	currentRowWidth := 0
	for _, span := range e.spansPerLines[e.cursor[0]][:e.cursor[1]] {
		currentRowWidth += span.width
	}

	blockOffset := 0
	if e.mode == ModeInsert || e.mode == ModeVLine || e.mode == ModeVisual || e.pendingAction == ActionVisual || e.pendingAction == ActionVisualLine {
		blockOffset = 1
	}
	targetRowX := 0
	targetRowWidth := 0
	targetRowSpans := e.spansPerLines[n]
	maxOffset := len(targetRowSpans) - 2 + blockOffset
	if maxOffset < 0 {
		maxOffset = 0
	}
	for _, span := range targetRowSpans[:maxOffset] {
		if span.runes == nil {
			break
		}
		if targetRowWidth+span.width > currentRowWidth {
			break
		}
		targetRowX++
		targetRowWidth += span.width
	}

	return [2]int{n, targetRowX}
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
	e.SetText(b.String(), from)
}

func (e *Editor) GetText(from, until [2]int) string {
	if from[0] > until[0] || from[0] == until[0] && from[1] > until[1] {
		from, until = until, from
	}

	var b strings.Builder
	lines := e.spansPerLines[from[0] : until[0]+1]
	for i, spans := range lines {
		for j, span := range spans {
			if i == 0 && j < from[1] {
				continue
			}
			if i == len(lines)-1 && j > until[1] {
				continue
			}

			if span.runes == nil {
				b.WriteString("\n")
				continue
			}
			b.WriteString(string(span.runes))
		}
	}

	return b.String()
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

	e.onDoneFunc(e, e.text)
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
	if e.undoOffset+2 > len(e.undoStack)-1 {
		return
	}
	n := e.getActionCount() + e.undoOffset + 1
	if n > len(e.undoStack)-1 {
		n = len(e.undoStack) - 1
	}
	redo := e.undoStack[n]
	e.undoOffset = n - 1
	e.SetText(redo.text, redo.cursor)
}

func (e *Editor) EnableSearch() [2]int {
	x, y, w, h := e.Box.GetInnerRect()
	se := New(WithKeymapper(e.keymapper)).SetOneLineMode(true)
	se.SetText("", [2]int{0, 0})
	se.SetRect(x, y+h-1, w, 1)
	se.SetDelayDrawFunc(e.delayDrawFunc)
	se.mode = ModeInsert
	se.onDoneFunc = func(_ *Editor, s string) {
		e.buildSearchIndexes('n', regexp.QuoteMeta(s), 0, 0, 0)
		e.operatorRunner[e.pendingAction](e.GetSearchCursor())
		e.searchEditor = nil
		e.ResetAction()
	}
	se.onExitFunc = func() {
		e.searchEditor = nil
		e.ResetAction()
	}
	e.searchEditor = se
	e.waitingForMotion = true
	return vim.AsyncMotion
}

func (e *Editor) Flash() [2]int {
	x, y, w, h := e.Box.GetInnerRect()
	se := New(WithKeymapper(e.keymapper)).SetOneLineMode(true)
	se.SetText("", [2]int{0, 0})
	se.SetRect(x, y+h-1, w, 1)
	se.SetDelayDrawFunc(e.delayDrawFunc)
	se.mode = ModeInsert
	se.onDoneFunc = func(_ *Editor, s string) {
		e.searchEditor = nil
		e.ResetAction()
		e.flashIndexes = make(map[rune][2]int)
		e.reverseFlashIndexes = make(map[[2]int]rune)
		e.motionIndexes['Z'] = nil
	}
	se.onTextChangedFunc = func(s string) {
		if len(s) < 1 {
			e.flashIndexes = make(map[rune][2]int)
			e.reverseFlashIndexes = make(map[[2]int]rune)
			e.motionIndexes['Z'] = nil
			return
		}

		if e.flashIndexes != nil && len(s) > e.flashIndexes['#'][0] {
			runes := []rune(s)
			r := runes[len(runes)-1]
			flash, hasFlash := e.flashIndexes[r]
			if hasFlash {
				e.operatorRunner[e.pendingAction](flash)
				e.searchEditor = nil
				e.ResetAction()
				e.flashIndexes = make(map[rune][2]int)
				e.reverseFlashIndexes = make(map[[2]int]rune)
				e.motionIndexes['Z'] = nil
			}
		}

		e.flashIndexes = make(map[rune][2]int)
		e.reverseFlashIndexes = make(map[[2]int]rune)
		// record last flash query len
		e.flashIndexes['#'] = [2]int{len(s), 0}
		e.buildSearchIndexes('Z', regexp.QuoteMeta(s), 0, e.offsets[0], e.offsets[0]+h-1)
		if e.motionIndexes['Z'] == nil {
			return
		}
		invalidFlash := make(map[rune]struct{})
		for _, index := range e.motionIndexes['Z'] {
			idx := index[2]
			if idx >= len(e.spansPerLines[index[0]])-2 {
				continue
			}
			invalidFlash[e.spansPerLines[index[0]][index[2]+1].runes[0]] = struct{}{}
		}
		flashIndexesClosestCursor := append([][3]int{}, e.motionIndexes['Z']...)
		sort.Slice(flashIndexesClosestCursor, func(i, j int) bool {
			xDistance1 := e.cursor[1] - flashIndexesClosestCursor[i][1]
			if xDistance1 < 0 {
				xDistance1 *= -1
			}
			yDistance1 := e.cursor[0] - flashIndexesClosestCursor[i][0]
			if yDistance1 < 0 {
				yDistance1 *= -1
			}

			xDistance2 := e.cursor[1] - flashIndexesClosestCursor[j][1]
			if xDistance2 < 0 {
				xDistance2 *= -1
			}
			yDistance2 := e.cursor[0] - flashIndexesClosestCursor[j][0]
			if yDistance2 < 0 {
				yDistance2 *= -1
			}

			return xDistance1+yDistance1 < xDistance2+yDistance2
		})

		i := 0
		for _, r := range flashAlphabet {
			if i > len(flashIndexesClosestCursor)-1 {
				break
			}
			_, invalid := invalidFlash[r]
			if invalid {
				continue
			}

			c := [2]int{flashIndexesClosestCursor[i][0], flashIndexesClosestCursor[i][1]}
			e.flashIndexes[r] = c
			e.reverseFlashIndexes[c] = r
			i++
		}
	}
	se.onExitFunc = func() {
		e.searchEditor = nil
		e.ResetAction()
		e.flashIndexes = make(map[rune][2]int)
		e.reverseFlashIndexes = make(map[[2]int]rune)
		e.motionIndexes['Z'] = nil
	}
	e.searchEditor = se
	e.waitingForMotion = true
	return vim.AsyncMotion
}

func (e *Editor) WaitingForMotion() [2]int {
	e.waitingForMotion = true
	return vim.AsyncMotion
}

func (e *Editor) AcceptRuneTil(r rune) {
	e.buildSearchIndexes('t', regexp.QuoteMeta(string(r)), -1, 0, 0)
}

func (e *Editor) AcceptRuneTilBack(r rune) {
	e.buildSearchIndexes('T', regexp.QuoteMeta(string(r)), 1, 0, 0)
}

func (e *Editor) AcceptRuneFind(r rune) {
	e.buildSearchIndexes('f', regexp.QuoteMeta(string(r)), 0, 0, 0)
}

func (e *Editor) AcceptRuneInside(r rune) {
	e.buildSurroundIndexes(r, true)
}

func (e *Editor) AcceptRuneAround(r rune) {
	e.buildSurroundIndexes(r, false)
}

func (e *Editor) buildSurroundIndexes(r rune, inside bool) {
	if r == 'w' {
		openingCursor, foundOpening := e.GetPrevMotionCursor('w', 1, e.cursor, true)
		closingCursor, foundClosing := e.GetNextMotionCursor('e', 1, e.cursor, true)
		if !foundOpening || !foundClosing {
			return
		}
		e.motionIndexes['s'] = [][3]int{
			{openingCursor[0], openingCursor[1], openingCursor[1]},
			{closingCursor[0], closingCursor[1], closingCursor[1]},
		}
		return
	}

	if r == 'W' {
		openingCursor, foundOpening := e.GetPrevMotionCursor('W', 1, e.cursor, true)
		closingCursor, foundClosing := e.GetNextMotionCursor('E', 1, e.cursor, true)
		if !foundOpening || !foundClosing {
			return
		}
		e.motionIndexes['s'] = [][3]int{
			{openingCursor[0], openingCursor[1], openingCursor[1]},
			{closingCursor[0], closingCursor[1], closingCursor[1]},
		}
		return
	}

	if !slices.Contains(matchBlocks, r) {
		return
	}

	if !slices.Contains(directionlessMatchBlocks, r) && matchBlockDirection[r] < 0 {
		r = matchingBlock[r]
	}
	e.buildSearchIndexes('s', regexp.QuoteMeta(string(r)), 0, 0, 0)
	if e.motionIndexes['s'] == nil {
		return
	}

	var openingCursor [2]int
	var closingCursor [2]int

	found := false
	i := 1
	left := true
	for range len(e.motionIndexes['s']) {
		if left {
			openingCursor, found = e.GetPrevMotionCursor('s', i, e.cursor, true)
		} else {
			openingCursor, found = e.GetNextMotionCursor('s', i, e.cursor, false)
		}

		// if not found on right side as well, then can early return
		if !left && !found {
			e.motionIndexes['s'] = nil
			return
		}

		// handle if there's no match block on the left side at all
		if left && !found {
			left = false
			i = 1
			continue
		}

		closingCursor = e.GetMatchingBlock(openingCursor)

		// if there's no matching block, then can early return
		if openingCursor == closingCursor {
			e.motionIndexes['s'] = nil
			return
		}

		// if still searching left and closing cursor before the current cursor, try different opening cursor
		if left && (closingCursor[0] < e.cursor[0] || (closingCursor[0] == e.cursor[0] && closingCursor[1] < e.cursor[1])) {
			newOpeningCursor, _ := e.GetPrevMotionCursor('s', i+1, e.cursor, false)

			// if new opening cursor is the same, then can search right
			if newOpeningCursor == openingCursor {
				left = false
				i = 1
				continue
			}
			i++
			continue
		}

		// valid, can break
		break
	}

	offset := 0
	if inside {
		offset = 1
	}
	e.motionIndexes['s'] = [][3]int{
		{openingCursor[0], openingCursor[1] + offset, openingCursor[1] + offset},
		{closingCursor[0], closingCursor[1] - offset, closingCursor[1] - offset},
	}
}

func (e *Editor) ChangeMode(m mode) {
	e.mode = m
}

func (e *Editor) DeleteUnderCursor() {
	n := e.getActionCount() + e.cursor[1]
	if n > len(e.spansPerLines[e.cursor[0]])-1 {
		n = len(e.spansPerLines[e.cursor[0]]) - 1
	}
	until := [2]int{e.cursor[0], n}
	e.ReplaceText("", e.cursor, until)
}

func (e *Editor) Undo() {
	if len(e.undoStack) < 1 {
		return
	}
	if e.undoOffset < 0 {
		return
	}
	n := e.undoOffset - e.getActionCount() + 1
	if n < 0 {
		n = 0
	}
	undo := e.undoStack[n]
	e.undoOffset = n - 1
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
	e.mode = ModeInsert
}

func (e *Editor) InsertAbove() {
	e.MoveCursorStartOfLine()
	e.ReplaceText("\n", e.cursor, e.cursor)
	e.cursor[1] = 0
	e.SaveChanges()
	e.undoOffset--
	e.mode = ModeInsert
}

func (e *Editor) ChangeUntil(until [2]int) {
	e.mode = ModeInsert
	e.DeleteUntil(until)
}

func (e *Editor) DeleteUntil(until [2]int) {
	from := e.cursor
	if until[0] < from[0] || (until[0] == from[0] && until[1] < from[1]) {
		from, until = until, from
	}
	clipboard.Write(e.GetText(from, until))
	e.ReplaceText("", from, until)
}

func (e *Editor) YankUntil(until [2]int) {
	e.VisualUntil(until)
	e.yankOnVisual = true
	if e.delayDrawFunc != nil {
		e.delayDrawFunc(time.Now().Add(100*time.Millisecond), func() {
			if e.yankOnVisual || e.mode == ModeVisual || e.mode == ModeVLine {
				e.yankOnVisual = false
				if e.mode != ModeVisual && e.mode != ModeVLine {
					return
				}

				e.mode = ModeNormal
				until := e.cursor
				from := e.visualStart
				if until[0] < from[0] || (until[0] == from[0] && until[1] < from[1]) {
					from, until = until, from
				}
				clipboard.Write(e.GetText(from, until))
				e.ResetMotionIndexes()
			}
		})
	}
}

func (e *Editor) VisualUntil(until [2]int) {
	if e.mode == ModeVisual {
		e.mode = ModeNormal
		return
	}

	e.visualStart = e.cursor
	e.MoveCursorTo(until)
	e.ChangeMode(ModeVisual)
}

func (e *Editor) ChangeUntilEndOfLine() {
	e.ChangeUntil(e.GetEndOfLineCursor())
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
	if len(e.spansPerLines) < 1 {
		return
	}

	from := [2]int{e.cursor[0], 0}
	until := [2]int{e.cursor[0] + 1, 0}
	if e.cursor[0] == len(e.spansPerLines)-1 {
		if e.cursor[0] != 0 {
			aboveRow := e.cursor[0] - 1
			from = [2]int{aboveRow, len(e.spansPerLines[aboveRow]) - 1}
		}
		until = [2]int{e.cursor[0], len(e.spansPerLines[e.cursor[0]]) - 1}
	}
	e.ReplaceText("", from, until)
	e.SaveChanges()
	e.undoOffset--
}

func (e *Editor) InsertAfter() {
	e.mode = ModeInsert
	e.MoveCursorRight()
}

func (e *Editor) InsertEndOfLine() {
	e.mode = ModeInsert
	e.MoveCursorEndOfLine()
}

func (e *Editor) MoveCursorFirstNonWhitespace() {
	e.MoveCursorTo(e.GetFirstNonWhitespaceCursor())
}

func (e *Editor) GetFirstNonWhitespaceCursor() [2]int {
	idx := rgFirstNonWhitespace.FindStringIndex(strings.Split(e.text, "\n")[e.cursor[0]])
	if len(idx) == 0 {
		return [2]int{e.cursor[0], 0}
	}

	return [2]int{e.cursor[0], idx[0]}
}

func (e *Editor) MoveMotion(motion rune, n int) {
	if n < 0 {
		e.cursor, _ = e.GetPrevMotionCursor(motion, n*-1, e.cursor, false)
		return
	}
	e.cursor, _ = e.GetNextMotionCursor(motion, n, e.cursor, false)
}

func (e *Editor) GetEndOfWordCursor() [2]int {
	c, _ := e.GetNextMotionCursor('e', e.getActionCount(), e.cursor, false)
	if e.pendingAction != ActionNone && e.pendingAction != ActionVisual && e.pendingAction != ActionYank {
		c[1]++
	}
	return c
}

func (e *Editor) GetStartOfWordCursor() [2]int {
	c, _ := e.GetNextMotionCursor('w', e.getActionCount(), e.cursor, false)
	return c
}

func (e *Editor) GetEndOfBigWordCursor() [2]int {
	c, _ := e.GetNextMotionCursor('E', e.getActionCount(), e.cursor, false)
	return c
}

func (e *Editor) GetBackEndOfBigWordCursor() [2]int {
	c, _ := e.GetPrevMotionCursor('E', e.getActionCount(), e.cursor, false)
	return c
}

func (e *Editor) GetStartOfBigWordCursor() [2]int {
	c, _ := e.GetNextMotionCursor('W', e.getActionCount(), e.cursor, false)
	return c
}

func (e *Editor) GetBackStartOfBigWordCursor() [2]int {
	c, _ := e.GetPrevMotionCursor('W', e.getActionCount(), e.cursor, false)
	return c
}

func (e *Editor) GetBackStartOfWordCursor() [2]int {
	c, _ := e.GetPrevMotionCursor('w', e.getActionCount(), e.cursor, false)
	return c
}

func (e *Editor) GetBackEndOfWordCursor() [2]int {
	c, _ := e.GetPrevMotionCursor('e', e.getActionCount(), e.cursor, false)
	return c
}

func (e *Editor) GetSearchCursor() [2]int {
	c, _ := e.GetNextMotionCursor('n', e.getActionCount(), e.cursor, false)
	return c
}

func (e *Editor) GetInsideOrAroundCursor() [2]int {
	if !e.waitingForMotion {
		return e.WaitingForMotion()
	}

	if e.motionIndexes['s'] == nil || len(e.motionIndexes['s']) != 2 {
		return e.cursor
	}

	mode := e.mode
	e.ChangeMode(ModeVisual)
	e.MoveCursorTo([2]int{e.motionIndexes['s'][0][0], e.motionIndexes['s'][0][1]})
	e.ChangeMode(mode)

	c := [2]int{e.motionIndexes['s'][1][0], e.motionIndexes['s'][1][1]}
	if e.pendingAction != ActionNone && e.pendingAction != ActionVisual && e.pendingAction != ActionYank {
		c[1]++
	}
	return c
}

func (e *Editor) GetTilCursor() [2]int {
	if !e.waitingForMotion {
		return e.WaitingForMotion()
	}

	c, found := e.GetNextMotionCursor('t', e.getActionCount(), e.cursor, false)
	if found && e.pendingAction != ActionNone && c != e.cursor && e.pendingAction != ActionVisual && e.pendingAction != ActionYank {
		c[1]++
	}
	return c
}

func (e *Editor) GetTilBackCursor() [2]int {
	if !e.waitingForMotion {
		return e.WaitingForMotion()
	}

	c, found := e.GetPrevMotionCursor('T', e.getActionCount(), e.cursor, false)
	if found && e.pendingAction != ActionNone && c != e.cursor && e.pendingAction != ActionVisual && e.pendingAction != ActionYank {
		c[1]++
	}
	return c
}

func (e *Editor) GetFindCursor() [2]int {
	if !e.waitingForMotion {
		return e.WaitingForMotion()
	}

	c, found := e.GetNextMotionCursor('f', e.getActionCount(), e.cursor, false)
	if found && e.pendingAction != ActionNone && c != e.cursor && e.pendingAction != ActionVisual && e.pendingAction != ActionYank {
		c[1]++
	}
	return c
}

func (e *Editor) GetFindBackCursor() [2]int {
	if !e.waitingForMotion {
		return e.WaitingForMotion()
	}

	c, _ := e.GetPrevMotionCursor('f', e.getActionCount(), e.cursor, false)
	return c
}

func (e *Editor) GetMatchingBlock(from [2]int) [2]int {
	if from[0] < 0 || from[0] > len(e.spansPerLines)-1 {
		return from
	}

	if from[1] < 0 || from[1] > len(e.spansPerLines[from[0]])-1 {
		return from
	}

	if e.spansPerLines[from[0]][from[1]].runes == nil {
		return from
	}
	r := e.spansPerLines[from[0]][from[1]].runes[0]
	if !slices.Contains(matchBlocks, r) {
		return from
	}

	if !slices.Contains(directionlessMatchBlocks, r) {
		direction := matchBlockDirection[r]
		n := 1
		spansPerLines := e.spansPerLines[from[0]:]
		if direction < 0 {
			spansPerLines = e.spansPerLines[:from[0]+1]
		}
		for i := range spansPerLines {
			first := i == 0
			if direction < 0 {
				i = len(spansPerLines) - 1 - i
			}
			for j := range spansPerLines[i] {
				if direction < 0 {
					j = len(spansPerLines[i]) - 1 - j
				}
				span := spansPerLines[i][j]
				if first && ((direction > 0 && j <= from[1]) || (direction < 0 && j >= from[1])) {
					continue
				}

				if span.runes != nil && span.runes[0] == r {
					n++
				}

				if span.runes != nil && span.runes[0] == matchingBlock[r] {
					n--
				}

				if n == 0 {
					if direction < 0 {
						i -= len(spansPerLines) - 1
						i *= -1
					}
					return [2]int{from[0] + (i * direction), j}
				}
			}
		}
		return from
	}

	e.buildSearchIndexes(r, string(r), 0, 0, 0)
	for i, index := range e.motionIndexes[r] {
		if index[0] == from[0] && index[1] == from[1] {
			target := i + 1
			if (i+1)%2 == 0 {
				target = i - 1
			}
			if target < 0 || target > len(e.motionIndexes[r])-1 {
				return from
			}
			return [2]int{e.motionIndexes[r][target][0], e.motionIndexes[r][target][1]}
		}
	}

	return from
}

func (e *Editor) searchDecorator(x, y, width, height int) {
	if e.motionIndexes['n'] == nil && e.motionIndexes['t'] == nil && e.motionIndexes['T'] == nil && e.motionIndexes['f'] == nil {
		return
	}

	indexes := e.motionIndexes['t']
	if indexes == nil {
		indexes = e.motionIndexes['T']
	}
	if indexes == nil {
		indexes = e.motionIndexes['f']
	}
	if indexes == nil {
		indexes = e.motionIndexes['n']
	}

	style1 := tcell.StyleDefault.Background(tview.Styles.ContrastBackgroundColor).Foreground(tview.Styles.PrimitiveBackgroundColor)
	style2 := tcell.StyleDefault.Background(tview.Styles.MoreContrastBackgroundColor).Foreground(tview.Styles.PrimitiveBackgroundColor)
	for _, idx := range indexes {
		if idx[0] < y {
			continue
		}
		if idx[0] >= y+height {
			break
		}

		for i := range idx[2] - idx[1] + 1 {
			if i == 0 && (e.motionIndexes['t'] != nil || e.motionIndexes['T'] != nil) {
				offset := -1
				if e.motionIndexes['t'] != nil {
					offset = 1
				}
				e.decorations[[2]int{idx[0], idx[1] + offset}] = decoration{style: style1, text: ""}
			}
			e.decorations[[2]int{idx[0], idx[1] + i}] = decoration{style: style2, text: ""}
		}
	}
}

func (e *Editor) flashDecorator(x, y, width, height int) {
	if e.motionIndexes['Z'] == nil {
		return
	}

	indexes := e.motionIndexes['Z']
	style1 := tcell.StyleDefault.Background(tview.Styles.MoreContrastBackgroundColor).Foreground(tview.Styles.PrimitiveBackgroundColor)
	style2 := tcell.StyleDefault.Background(tview.Styles.ContrastBackgroundColor).Foreground(tview.Styles.PrimitiveBackgroundColor)
	for _, idx := range indexes {
		if idx[0] < y {
			continue
		}
		if idx[0] >= y+height {
			break
		}

		for i := range idx[2] - idx[1] + 1 {
			e.decorations[[2]int{idx[0], idx[1] + i}] = decoration{style: style2, text: ""}
		}
	}

	for _, idx := range indexes {
		if idx[0] < y {
			continue
		}
		if idx[0] >= y+height {
			break
		}

		r, hasFlash := e.reverseFlashIndexes[[2]int{idx[0], idx[1]}]
		if hasFlash {
			e.decorations[[2]int{idx[0], idx[2] + 1}] = decoration{style: style1, text: string(r)}
		}
	}
}

func (e *Editor) visualDecorator(x, y, width, height int) {
	if e.mode != ModeVisual && e.mode != ModeVLine {
		return
	}

	from := e.visualStart
	until := e.cursor
	if from[0] > until[0] || from[0] == until[0] && from[1] > until[1] {
		from, until = until, from
	}

	style := tcell.StyleDefault.Background(tview.Styles.MoreContrastBackgroundColor).Foreground(tview.Styles.PrimitiveBackgroundColor)
	for row := range until[0] - from[0] + 1 {
		row += from[0]
		lineWidth := 0
		if row < y {
			continue
		}
		if row >= y+height {
			break
		}

		for col, span := range e.spansPerLines[row] {
			lineWidth += span.width
			if lineWidth < x {
				continue
			}
			if lineWidth > width {
				break
			}

			if (e.mode == ModeVisual &&
				(row == from[0] && col >= from[1] && row == until[0] && col <= until[1]) ||
				(row == from[0] && row < until[0] && col >= from[1]) ||
				(row > from[0] && row < until[0]) || (row == until[0] && row > from[0] && col <= until[1])) ||
				(e.mode == ModeVLine &&
					(row >= from[0] && row <= until[0])) {
				e.decorations[[2]int{row, col}] = decoration{style: style, text: ""}
			}
		}
	}
}

func (e *Editor) highlightDecorator(x, y, width, height int) {
	byte := 0
	byteMapper := make(map[int][2]int)
	for row, spans := range e.spansPerLines {
		for col, span := range spans {
			for i := range span.bytesWidth {
				byteMapper[byte+i] = [2]int{row, col}
			}
			byte += span.bytesWidth
		}
		byte += 1
	}

	for byteRange, kind := range e.highlightIndexes {
		style, hasStyle := colorMap[kind]
		if !hasStyle {
			continue
		}

		for i := range byteRange[1] - byteRange[0] {
			c := byteMapper[i+byteRange[0]]
			e.decorations[c] = decoration{style: style, text: ""}

			if kind == "error" {
				e.decorations[[2]int{c[0], len(e.spansPerLines[c[0]]) - 1}] = decoration{style: tcell.StyleDefault.Foreground(tcell.ColorRed).Underline(tcell.UnderlineStyleCurly, tcell.ColorRed), text: "     syntax error"}
			}
		}
	}
}

func (e *Editor) ResetMotionIndexes() {
	e.motionIndexes['n'] = nil
	e.motionIndexes['t'] = nil
	e.motionIndexes['T'] = nil
	e.motionIndexes['f'] = nil
	e.motionIndexes['Z'] = nil
}

func (e *Editor) ResetAction() {
	e.pendingAction = ActionNone
	e.lastMotion = ActionNone
	e.pending = nil
	e.pendingCount = 0
	e.waitingForMotion = false
}

func WriteFile(text string) {
	f, err := os.Create("~/repos/sqluy/" + strconv.Itoa(int(time.Now().UnixMilli())))
	if err != nil {
		panic(err)
	}

	fmt.Fprint(f, text)
}
