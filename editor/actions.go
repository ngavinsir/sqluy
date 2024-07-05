package editor

import (
	"slices"
	"sync"
)

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
	ActionInside
	ActionAround
	ActionDeleteUntilEndOfLine
	ActionDeleteLine
	ActionReplace
	ActionPasteAfter
	ActionPasteBefore
	ActionMoveLastLine
	ActionMoveFirstLine
	ActionMoveEndOfWord
	ActionMoveStartOfWord
	ActionMoveBackStartOfWord
	ActionMoveBackEndOfWord
	ActionMoveStartOfBigWord
	ActionMoveBackStartOfBigWord
	ActionMoveEndOfBigWord
	ActionMoveBackEndOfBigWord
	ActionMoveNextSearch
	ActionMovePrevSearch
	ActionMoveNextFind
	ActionMovePrevFind
	ActionMoveMatchBlock
	ActionSwitchVisualStart
	ActionFlash
	ActionVisual
	ActionVisualLine
	ActionTil
	ActionTilBack
	ActionFind
	ActionFindBack
	ActionExit
	ActionChange
	ActionDelete
	ActionYank
)

var OperatorActions = []Action{ActionChange, ActionDelete, ActionYank, ActionVisual}
var MotionActions = []Action{ActionMoveLeft, ActionMoveRight, ActionMoveUp, ActionMoveDown, ActionMoveEndOfLine, ActionMoveStartOfLine, ActionMoveFirstNonWhitespace, ActionFlash,
	ActionMoveLastLine, ActionMoveFirstLine, ActionMoveEndOfWord, ActionMoveStartOfWord, ActionMoveBackStartOfWord, ActionMoveBackEndOfWord, ActionEnableSearch, ActionTil,
	ActionTilBack, ActionFind, ActionFindBack, ActionInside, ActionAround, ActionMoveStartOfBigWord, ActionMoveBackStartOfBigWord, ActionMoveEndOfBigWord, ActionMoveBackEndOfBigWord}
var CountlessMotionActions = []Action{ActionMoveStartOfLine}
var OperatorlessMotionActions = []Action{ActionMoveLeft, ActionMoveRight, ActionMoveUp, ActionMoveDown, ActionMoveEndOfLine, ActionMoveStartOfLine, ActionMoveFirstNonWhitespace,
	ActionMoveLastLine, ActionMoveFirstLine, ActionMoveEndOfWord, ActionMoveStartOfWord, ActionMoveBackStartOfWord, ActionMoveBackEndOfWord, ActionEnableSearch, ActionTil,
	ActionTilBack, ActionFind, ActionFindBack, ActionMoveStartOfBigWord, ActionMoveBackStartOfBigWord, ActionMoveEndOfBigWord, ActionMoveBackEndOfBigWord, ActionFlash}
var WaitingForRuneActions = []Action{ActionTil, ActionTilBack, ActionFind, ActionFindBack, ActionInside, ActionAround}

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
	ActionInside:                 "inside",
	ActionAround:                 "around",
	ActionFlash:                  "flash",
	ActionDeleteUntilEndOfLine:   "delete_until_end_of_line",
	ActionDeleteLine:             "delete_line",
	ActionReplace:                "replace",
	ActionPasteAfter:             "paste_after",
	ActionPasteBefore:            "paste_before",
	ActionMoveLastLine:           "move_last_line",
	ActionMoveFirstLine:          "move_first_line",
	ActionMoveEndOfWord:          "move_end_of_word",
	ActionMoveStartOfWord:        "move_start_of_word",
	ActionMoveBackStartOfWord:    "move_back_start_of_word",
	ActionMoveStartOfBigWord:     "move_start_of_big_word",
	ActionMoveBackStartOfBigWord: "move_back_start_of_big_word",
	ActionMoveEndOfBigWord:       "move_end_of_big_word",
	ActionMoveBackEndOfBigWord:   "move_back_end_of_big_word",
	ActionMoveBackEndOfWord:      "move_back_end_of_word",
	ActionMoveNextSearch:         "move_next_search",
	ActionMovePrevSearch:         "move_prev_search",
	ActionMoveNextFind:           "move_next_find",
	ActionMovePrevFind:           "move_prev_find",
	ActionMoveMatchBlock:         "move_match_block",
	ActionSwitchVisualStart:      "switch_visual_start",
	ActionVisual:                 "visual",
	ActionVisualLine:             "visual_line",
	ActionTil:                    "til",
	ActionTilBack:                "til_back",
	ActionFind:                   "find",
	ActionFindBack:               "find_back",
	ActionExit:                   "exit",
	ActionChange:                 "change",
	ActionDelete:                 "delete",
	ActionYank:                   "yank",
}
var reverseActionMapper map[string]Action
var reverseActionMapperOnce sync.Once

func (a Action) String() string {
	if actionMapper[a] != "" {
		return "editor." + actionMapper[a]
	}
	return "editor.none"
}

func (a Action) IsOperator() bool {
	return slices.Contains(OperatorActions, a)
}

func (a Action) IsMotion() bool {
	return slices.Contains(MotionActions, a)
}

func (a Action) IsOperatorlessMotion() bool {
	return slices.Contains(OperatorlessMotionActions, a)
}

func (a Action) IsCountlessMotion() bool {
	return slices.Contains(CountlessMotionActions, a)
}

func (a Action) IsWaitingForRune() bool {
	return slices.Contains(WaitingForRuneActions, a)
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
