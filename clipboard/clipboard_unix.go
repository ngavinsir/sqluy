// Modification of https://github.com/atotto/clipboard/blob/bdea50a7aaf00a87beb042c9965cf6b1090cdf4e/clipboard_unix.go

// Copyright 2013 @atotto. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build freebsd || linux || netbsd || openbsd || solaris || dragonfly
// +build freebsd linux netbsd openbsd solaris dragonfly

package clipboard

import (
	"errors"
	"os"
	"os/exec"
	"sync"
)

const (
	xsel               = "xsel"
	xclip              = "xclip"
	powershellExe      = "powershell.exe"
	clipExe            = "clip.exe"
	wlcopy             = "wl-copy"
	wlpaste            = "wl-paste"
	termuxClipboardGet = "termux-clipboard-get"
	termuxClipboardSet = "termux-clipboard-set"
)

var (
	once         sync.Once
	pasteCmdArgs []string
	copyCmdArgs  []string
	trimDos      bool

	xselPasteArgs = []string{xsel, "--output", "--clipboard"}
	xselCopyArgs  = []string{xsel, "--input", "--clipboard"}

	xclipPasteArgs = []string{xclip, "-out", "-selection", "clipboard"}
	xclipCopyArgs  = []string{xclip, "-in", "-selection", "clipboard"}

	powershellExePasteArgs = []string{powershellExe, "Get-Clipboard"}
	clipExeCopyArgs        = []string{clipExe}

	wlpasteArgs = []string{wlpaste, "--no-newline"}
	wlcopyArgs  = []string{wlcopy}

	termuxPasteArgs = []string{termuxClipboardGet}
	termuxCopyArgs  = []string{termuxClipboardSet}

	errUnsupported = errors.New("no clipboard utilities available. Please install xsel, xclip, wl-clipboard or Termux:API add-on for termux-clipboard-get/set")
)

func setCmdArgs() {
	once.Do(func() {
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			if _, err := exec.LookPath(wlcopy); err == nil {
				if _, err := exec.LookPath(wlpaste); err == nil {
					copyCmdArgs = wlcopyArgs
					pasteCmdArgs = wlpasteArgs
					return
				}
			}
		}

		if _, err := exec.LookPath(xclip); err == nil {
			copyCmdArgs = xclipCopyArgs
			pasteCmdArgs = xclipPasteArgs
			return
		}

		if _, err := exec.LookPath(xsel); err == nil {
			copyCmdArgs = xselCopyArgs
			pasteCmdArgs = xselPasteArgs
			return
		}

		if _, err := exec.LookPath(termuxClipboardSet); err == nil {
			if _, err := exec.LookPath(termuxClipboardGet); err == nil {
				copyCmdArgs = termuxCopyArgs
				pasteCmdArgs = termuxPasteArgs
				return
			}
		}

		if _, err := exec.LookPath(clipExe); err == nil {
			if _, err := exec.LookPath(powershellExe); err == nil {
				trimDos = true
				copyCmdArgs = clipExeCopyArgs
				pasteCmdArgs = powershellExePasteArgs
				return
			}
		}
	})
}

func getPasteCommand() *exec.Cmd {
	return exec.Command(pasteCmdArgs[0], pasteCmdArgs[1:]...)
}

func getCopyCommand() *exec.Cmd {
	return exec.Command(copyCmdArgs[0], copyCmdArgs[1:]...)
}

func readAll() (string, error) {
	setCmdArgs()
	if pasteCmdArgs == nil {
		return "", errUnsupported
	}
	pasteCmd := getPasteCommand()
	out, err := pasteCmd.Output()
	if err != nil {
		return "", err
	}
	result := string(out)
	if trimDos && len(result) > 1 {
		result = result[:len(result)-2]
	}
	return result, nil
}

func writeAll(text string) error {
	setCmdArgs()
	if copyCmdArgs == nil {
		return errUnsupported
	}
	copyCmd := getCopyCommand()
	in, err := copyCmd.StdinPipe()
	if err != nil {
		return err
	}

	if err := copyCmd.Start(); err != nil {
		return err
	}
	if _, err := in.Write([]byte(text)); err != nil {
		return err
	}
	if err := in.Close(); err != nil {
		return err
	}
	return copyCmd.Wait()
}
