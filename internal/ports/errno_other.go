//go:build !windows

package ports

import "syscall"

// The errors of a listen on a busy port and on a port the system refuses.
const (
	errInUse     = syscall.EADDRINUSE
	errForbidden = syscall.EACCES
)
