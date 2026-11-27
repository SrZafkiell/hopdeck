//go:build windows

package ports

import "syscall"

// The Winsock errors of a listen on a busy port and on a port the system refuses.
const (
	errInUse     = syscall.Errno(10048) // WSAEADDRINUSE
	errForbidden = syscall.Errno(10013) // WSAEACCES
)
