package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// enableANSI turns on colour escape codes in the Windows console (Windows
// 10+). Returns false if the console can't do it, so we skip the QR code.
func enableANSI() bool {
	h := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return false
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true
	}
	return windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
