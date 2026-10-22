//go:build !windows && !linux && !freebsd && !openbsd && !netbsd

package autostart

func enabled() (bool, error) {
	return false, errUnsupported
}

func enableAutostart() error {
	return errUnsupported
}

func disableAutostart() error {
	return errUnsupported
}
