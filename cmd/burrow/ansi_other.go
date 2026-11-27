//go:build !windows

package main

// enableANSI: Unix terminals understand colour escape codes already.
func enableANSI() bool { return true }
