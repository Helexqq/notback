package util

import (
	"fmt"
	"runtime"
)

func Wrap(err error, args ...any) error {
	if err == nil {
		return nil
	}
	pc, _, _, _ := runtime.Caller(1)
	fn := runtime.FuncForPC(pc).Name()
	return fmt.Errorf("%s%v: %w", fn, args, err)
}
