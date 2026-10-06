package neomock

import (
	"fmt"
	"runtime"
)

// Caller возвращает "file:line" вызывающего кадра для origin в сообщениях.
// Фиксируется в When/Run (а не в точке вызова), чтобы диагностика из
// Cleanup/Verify указывала на место регистрации ожидания в тесте.
// skip — число кадров выше вызывающего (1 — непосредственный вызывающий).
func Caller(skip int) string {
	_, file, line, ok := runtime.Caller(skip + 1)
	if !ok {
		return "<unknown>"
	}
	return fmt.Sprintf("%s:%d", file, line)
}
