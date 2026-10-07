//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// hideConsoleIfOwned skryje černé okno, pokud bylo otevřeno dvojklikem jen pro tento program
// (v příkazovém řádku, který program spustil, zůstává okno beze změny).
func hideConsoleIfOwned() {
	k := syscall.NewLazyDLL("kernel32.dll")
	var pids [4]uint32
	n, _, _ := k.NewProc("GetConsoleProcessList").Call(uintptr(unsafe.Pointer(&pids[0])), 4)
	if n == 1 {
		k.NewProc("FreeConsole").Call()
	}
}
