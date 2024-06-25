package keymap

import (
	"encoding/json"
	"strings"
)

type (
	keymapJSON struct {
		Keymaps []struct {
			Keys   []string `json:"keys"`
			Groups []string `json:"groups"`
			Action string   `json:"action"`
		} `json:"keymaps"`
	}

	keyTree struct {
		childs map[string]*keyTree
		action string
	}

	Keymapper struct {
		keyTreePerGroup map[string]*keyTree
	}
)

func (k *keyTree) Add(keys []string, action string) {
	if k.childs == nil {
		k.childs = make(map[string]*keyTree)
	}
	if len(keys) == 1 {
		k.childs[keys[0]] = &keyTree{action: action}
		return
	}
	k.childs[keys[0]] = &keyTree{}
	k.childs[keys[0]].Add(keys[1:], action)
}

func (k *keyTree) Get(keys []string) (string, bool) {
	if k == nil {
		return "", false
	}
	if len(keys) == 0 {
		return k.action, k.childs != nil && len(k.childs) > 0
	}
	return k.childs[keys[0]].Get(keys[1:])
}

func (k *keyTree) String() string {
	if k.action != "" {
		return k.action
	}
	var b strings.Builder
	for k, c := range k.childs {
		b.WriteString(k + "\n " + strings.Join(strings.Split(c.String(), "\n"), "\n ") + "\n")
	}
	return b.String()
}

func New(s string) Keymapper {
	k := Keymapper{keyTreePerGroup: keyTreePerGroupFromJSONString(s)}
	return k
}

func keyTreePerGroupFromJSONString(s string) map[string]*keyTree {
	m := make(map[string]*keyTree)

	var j keymapJSON
	err := json.Unmarshal([]byte(s), &j)
	if err != nil {
		panic("invalid key map json: " + err.Error())
	}

	for _, keymap := range j.Keymaps {
		for _, group := range keymap.Groups {
			if m[group] == nil {
				m[group] = &keyTree{}
			}
			m[group].Add(keymap.Keys, keymap.Action)
		}
	}
	return m
}

func (k Keymapper) Get(keys []string, group string) (string, bool) {
	if k.keyTreePerGroup[group] == nil {
		return "", false
	}
	return k.keyTreePerGroup[group].Get(keys)
}