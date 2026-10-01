//go:build windows

package main

import (
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// pickOpenPath shows the native Windows open dialog (comdlg32). Returns
// ok=false when the user cancels.
func pickOpenPath(def string) (string, bool) {
	const (
		ofnNoChangeDir   = 0x8
		ofnPathMustExist = 0x800
		ofnFileMustExist = 0x1000
		bufLen           = 4096
	)
	comdlg32 := syscall.NewLazyDLL("comdlg32.dll")
	proc := comdlg32.NewProc("GetOpenFileNameW")

	filter := utf16.Encode([]rune("Text Files (*.txt)\x00*.txt\x00All Files (*.*)\x00*\x00\x00"))
	buf := make([]uint16, bufLen)
	name := utf16.Encode([]rune(def))
	copy(buf, name)

	var ofn struct {
		lStructSize       uint32
		hwndOwner         uintptr
		hInstance         uintptr
		lpstrFilter       *uint16
		lpstrCustomFilter *uint16
		nMaxCustFilter    uint32
		nFilterIndex      uint32
		lpstrFile         *uint16
		nMaxFile          uint32
		lpstrFileTitle    *uint16
		nMaxFileTitle     uint32
		lpstrInitialDir   *uint16
		lpstrTitle        *uint16
		flags             uint32
		nFileOffset       uint16
		nFileExtension    uint16
		lpstrDefExt       *uint16
		lCustData         uintptr
		lpfnHook          uintptr
		lpTemplateName    *uint16
		pvReserved        uintptr
		dwReserved        uint32
		dwFlagsEx         uint32
	}
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	ofn.lpstrFilter = &filter[0]
	ofn.lpstrFile = &buf[0]
	ofn.nMaxFile = bufLen
	ofn.flags = ofnNoChangeDir | ofnPathMustExist | ofnFileMustExist

	r, _, _ := proc.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		return "", false // cancel (or dialog error: treated as cancel)
	}
	out := make([]uint16, 0, bufLen)
	for _, u := range buf {
		if u == 0 {
			break
		}
		out = append(out, u)
	}
	path := string(utf16.Decode(out))
	if path == "" {
		return "", false
	}
	return path, true
}

// pickSavePath shows the native Windows save dialog (comdlg32). Returns
// ok=false when the user cancels.
func pickSavePath(def string) (string, bool) {
	const (
		ofnOverwritePrompt = 0x2
		ofnNoChangeDir     = 0x8
		ofnPathMustExist   = 0x800
		bufLen             = 4096
	)
	comdlg32 := syscall.NewLazyDLL("comdlg32.dll")
	proc := comdlg32.NewProc("GetSaveFileNameW")

	filter := utf16.Encode([]rune("Text Files (*.txt)\x00*.txt\x00All Files (*.*)\x00*\x00\x00"))
	buf := make([]uint16, bufLen)
	name := utf16.Encode([]rune(def))
	copy(buf, name)

	var ofn struct {
		lStructSize       uint32
		hwndOwner         uintptr
		hInstance         uintptr
		lpstrFilter       *uint16
		lpstrCustomFilter *uint16
		nMaxCustFilter    uint32
		nFilterIndex      uint32
		lpstrFile         *uint16
		nMaxFile          uint32
		lpstrFileTitle    *uint16
		nMaxFileTitle     uint32
		lpstrInitialDir   *uint16
		lpstrTitle        *uint16
		flags             uint32
		nFileOffset       uint16
		nFileExtension    uint16
		lpstrDefExt       *uint16
		lCustData         uintptr
		lpfnHook          uintptr
		lpTemplateName    *uint16
		pvReserved        uintptr
		dwReserved        uint32
		dwFlagsEx         uint32
	}
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	ofn.lpstrFilter = &filter[0]
	ofn.lpstrFile = &buf[0]
	ofn.nMaxFile = bufLen
	ofn.flags = ofnOverwritePrompt | ofnNoChangeDir | ofnPathMustExist

	r, _, _ := proc.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		return "", false // cancel (or dialog error: treated as cancel)
	}
	// Decode the NUL-terminated UTF-16 result.
	out := make([]uint16, 0, bufLen)
	for _, u := range buf {
		if u == 0 {
			break
		}
		out = append(out, u)
	}
	path := string(utf16.Decode(out))
	if path == "" {
		return "", false
	}
	return path, true
}
