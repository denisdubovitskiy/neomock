package neomock

import "fmt"

// maxFormattedValue — усечение значения в сообщениях мока (DESIGN 3.2.4:
// %#v с усечением до 500 символов, чтобы длинные []byte/struct не топили журнал).
const maxFormattedValue = 500

// FormatValue форматирует значение для сообщений мока: %#v с усечением
// до 500 символов. Используется сгенерированным кодом.
func FormatValue(v any) string {
	return truncate(fmt.Sprintf("%#v", v), maxFormattedValue)
}

// FormatExpected форматирует ожидаемое значение из зарегистрированного When
// для сообщений мока: маркер neomock.Context — читабельным литералом,
// остальное — как FormatValue (%#v с усечением). Используется сгенерированным
// кодом в дампах ожиданий и mismatch-репортах (DESIGN 3.2.4).
func FormatExpected(v any) string {
	if IsAnyContext(v) {
		return "neomock.Context"
	}
	return FormatValue(v)
}

// FormatArgs форматирует список аргументов вызова как "name=..., name=...".
// Имена и значения передаёт генерат (формат сообщений — DESIGN 3.2.4).
func FormatArgs(names []string, values []any) string {
	s := ""
	for i, v := range values {
		if i > 0 {
			s += ", "
		}
		if i < len(names) {
			s += names[i] + "="
		}
		s += FormatValue(v)
	}
	return s
}
