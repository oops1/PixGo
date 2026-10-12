//go:build !windows

package platform

type foregrounder interface {
	SetForeground()
}

func RaiseWindow(native any) error {
	fg, ok := native.(foregrounder)
	if !ok {
		return ErrUnsupported
	}
	fg.SetForeground()
	return nil
}
