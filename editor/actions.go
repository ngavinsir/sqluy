package editor

import "sync"

type Action uint64

const (
	ActionNone Action = iota
	ActionMoveLeft
	ActionMoveRight
	ActionMoveUp
	ActionMoveDown
	ActionDone
	ActionEnableSearch
	ActionInsert
	ActionRedo
	ActionUndo
	ActionMoveHalfPageUp
	ActionMoveHalfPageDown
	ActionDeleteUnderCursor
	ActionInsertAfter
	ActionInsertEndOfLine
	ActionMoveEndOfLine
	ActionMoveStartOfLine
	ActionMoveFirstNonWhitespace
	ActionInsertBelow
	ActionInsertAbove
	ActionChangeUntilEndOfLine
	ActionDeleteUntilEndOfLine
	ActionDeleteLine
	ActionReplace
	ActionMoveLastLine
	ActionMoveFirstLine
	ActionMoveEndOfWord
	ActionMoveStartOfWord
	ActionMoveBackStartOfWord
	ActionMoveBackEndOfWord
	ActionMoveNextSearch
	ActionMovePrevSearch
	ActionExit
)

var actionMapper = map[Action]string{
	ActionMoveLeft:               "move_left",
	ActionMoveRight:              "move_right",
	ActionMoveUp:                 "move_up",
	ActionMoveDown:               "move_down",
	ActionDone:                   "done",
	ActionEnableSearch:           "enable_search",
	ActionInsert:                 "insert",
	ActionRedo:                   "redo",
	ActionUndo:                   "undo",
	ActionMoveHalfPageUp:         "move_half_page_up",
	ActionMoveHalfPageDown:       "move_half_page_down",
	ActionDeleteUnderCursor:      "delete_under_cursor",
	ActionInsertAfter:            "insert_after",
	ActionInsertEndOfLine:        "insert_end_of_line",
	ActionMoveEndOfLine:          "move_end_of_line",
	ActionMoveStartOfLine:        "move_start_of_line",
	ActionMoveFirstNonWhitespace: "move_first_non_whitespace",
	ActionInsertBelow:            "insert_below",
	ActionInsertAbove:            "insert_above",
	ActionChangeUntilEndOfLine:   "change_until_end_of_line",
	ActionDeleteUntilEndOfLine:   "delete_until_end_of_line",
	ActionDeleteLine:             "delete_line",
	ActionReplace:                "replace",
	ActionMoveLastLine:           "move_last_line",
	ActionMoveFirstLine:          "move_first_line",
	ActionMoveEndOfWord:          "move_end_of_word",
	ActionMoveStartOfWord:        "move_start_of_word",
	ActionMoveBackStartOfWord:    "move_back_start_of_word",
	ActionMoveBackEndOfWord:      "move_back_end_of_word",
	ActionMoveNextSearch:         "move_next_search",
	ActionMovePrevSearch:         "move_prev_search",
	ActionExit:                   "exit",
}
var reverseActionMapper map[string]Action
var reverseActionMapperOnce sync.Once

func (a Action) String() string {
	if actionMapper[a] != "" {
		return "editor." + actionMapper[a]
	}
	return "editor.none"
}

func ActionFromString(s string) Action {
	reverseActionMapperOnce.Do(func() {
		reverseActionMapper = make(map[string]Action, len(actionMapper))
		for k, v := range actionMapper {
			reverseActionMapper["editor."+v] = k
		}
	})

	return reverseActionMapper[s]
}
