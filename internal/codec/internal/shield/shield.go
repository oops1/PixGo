package shield

import (
	"github.com/oops1/PixGo/internal/codec/internal/guard"
)

func Run(format string, call func() error) (err error) {
	defer func() {
		if value := recover(); value != nil {
			err = guard.Corrupt("%s: decoder panic: %v", format, value)
		}
	}()
	return call()
}
