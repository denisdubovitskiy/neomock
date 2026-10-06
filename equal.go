package neomock

import (
	"bytes"
	"context"
	"reflect"
)

// Equal сравнивает expected и actual:
//   - если expected — маркер neomock.Context, а actual реализует context.Context
//     и не nil → true (wildcard — единственное расширение относительно testify);
//   - иначе как require.Equal: bytes.Equal для []byte (nil равен пустому),
//     reflect.DeepEqual для остального.
//
// Логика завендорена (~25 строк), импорта testify нет. Никаких собственных
// матчеров и «умных» правил сверх этого нет (см. DESIGN 3.2.4).
func Equal(expected, actual any) bool {
	if IsAnyContext(expected) {
		if actual == nil {
			return false
		}
		_, ok := actual.(context.Context)
		return ok
	}
	if e, ok := expected.([]byte); ok {
		a, ok := actual.([]byte)
		if !ok {
			return false
		}
		return bytes.Equal(e, a)
	}
	return reflect.DeepEqual(expected, actual)
}
