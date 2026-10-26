// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package update

import (
	"syscall"
	"unsafe"
)

// localeNameMax is LOCALE_NAME_MAX_LENGTH, the room a locale name needs with its NUL
const localeNameMax = 85

var getUserDefaultLocaleName = syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultLocaleName")

// SystemLocale is the user's locale, normalized (see NormalizeLocale): the environment's
// when it names one (Git Bash, MSYS), else Windows' own user locale
func SystemLocale() string {
	if l := NormalizeLocale(envLocale()); l != "" {
		return l
	}

	return NormalizeLocale(windowsLocale())
}

// windowsLocale asks Windows for the user's locale name, "" when it can't say
func windowsLocale() string {
	buf := make([]uint16, localeNameMax)
	n, _, _ := getUserDefaultLocaleName.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return ""
	}

	return syscall.UTF16ToString(buf)
}
