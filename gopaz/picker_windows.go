//go:build windows

package main

import (
	"runtime"
	"sync"

	winfilepicker "github.com/zyoung11/GO-WinFilePicker"
)

var pickerMu sync.Mutex

func pickVideoFile() (string, error) {
	pickerMu.Lock()
	defer pickerMu.Unlock()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	return winfilepicker.SelectFile("Select Video", videoExtensions...)
}

func pickOutputFolder() (string, error) {
	pickerMu.Lock()
	defer pickerMu.Unlock()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	return winfilepicker.SelectFolder("Select Output Folder")
}
