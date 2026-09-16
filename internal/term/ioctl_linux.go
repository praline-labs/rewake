package term

// The ioctl numbers are architecture-independent on Linux.
const (
	tcgets    = 0x5401
	tiocgpgrp = 0x540f
	tiocspgrp = 0x5410
)
