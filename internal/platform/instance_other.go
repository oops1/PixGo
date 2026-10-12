//go:build !windows && !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package platform

type instanceSys struct{}

func (*instanceSys) acquire(string) (bool, error) { return true, nil }
func (*instanceSys) send(string, []string) error  { return ErrUnavailable }
func (*instanceSys) listen(func([]string)) error  { return nil }
func (*instanceSys) close() error                 { return nil }
