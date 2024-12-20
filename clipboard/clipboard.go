// Modification of https://github.com/atotto/clipboard/blob/a77150d852d0f278f1068b1befce317c97f3a2e5/clipboard.go

// Copyright 2013 @atotto. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package clipboard read/write on clipboard
package clipboard

// ReadAll read string from clipboard
func Read() (string, error) {
	return read()
}

// WriteAll write string to clipboard
func Write(text string) error {
	return write(text)
}
