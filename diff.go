package neomock

import (
	"fmt"
	"reflect"
	"strings"
)

// Максимум строк, символов и размер одного значения в выводе Diff —
// чтобы не топить журнал теста.
const (
	diffMaxLines   = 20
	diffMaxRunes   = 2000
	diffMaxElement = 200
)

// Diff возвращает человекочитаемый построчный diff «ожидалось/получено»:
// struct — по полям, slice/map — поэлементно, иначе — fmt %#v обоих значений.
// Пустая строка, если значения равны. Вывод усечён до ~20 строк / 2000
// символов. Используется в mismatch-сообщениях мока перед %#v-простынями
// (см. DESIGN 3.2.4).
func Diff(expected, actual any) string {
	if Equal(expected, actual) {
		return ""
	}
	var b strings.Builder
	switch reflect.TypeOf(expected).Kind() {
	case reflect.Struct:
		av := reflect.ValueOf(actual)
		if av.IsValid() && av.Type() == reflect.TypeOf(expected) {
			diffStruct(&b, "", reflect.ValueOf(expected), av)
		} else {
			fmt.Fprintf(&b, "expected %#v, got %#v", expected, actual)
		}
	case reflect.Slice:
		if isByteSlice(expected) {
			fmt.Fprintf(
				&b,
				"expected %s, got %s",
				shortBytes(toBytes(expected)),
				shortBytes(toBytes(actual)),
			)
		} else {
			diffSlice(&b, reflect.ValueOf(expected), reflect.ValueOf(actual))
		}
	case reflect.Map:
		diffMap(&b, reflect.ValueOf(expected), reflect.ValueOf(actual))
	default:
		fmt.Fprintf(&b, "expected %#v, got %#v", expected, actual)
	}
	return truncateDiff(b.String())
}

// diffStruct добавляет построчное сравнение полей двух структур одного типа.
func diffStruct(
	b *strings.Builder,
	prefix string,
	expected,
	actual reflect.Value,
) {
	t := expected.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		path := prefix + "." + f.Name
		ev, av := expected.Field(i), actual.Field(i)
		if reflect.DeepEqual(ev.Interface(), av.Interface()) {
			continue
		}
		switch ev.Kind() {
		case reflect.Struct:
			diffStruct(b, path, ev, av)
		case reflect.Slice:
			if !isByteValue(ev) {
				diffSliceAt(b, path, ev, av)
				continue
			}
			fmt.Fprintf(
				b,
				"Field %s: expected %s, got %s\n",
				path,
				shortValue(ev),
				shortValue(av),
			)
		case reflect.Map:
			diffMapAt(b, path, ev, av)
		default:
			fmt.Fprintf(
				b,
				"Field %s: expected %s, got %s\n",
				path,
				shortValue(ev),
				shortValue(av),
			)
		}
	}
}

// diffSlice добавляет поэлементное сравнение слайсов.
func diffSlice(b *strings.Builder, expected, actual reflect.Value) {
	diffSliceAt(b, "[]", expected, actual)
}

// diffSliceAt сравнивает слайсы по индексам под общим префиксом пути.
func diffSliceAt(
	b *strings.Builder,
	prefix string,
	expected,
	actual reflect.Value,
) {
	if expected.Kind() != reflect.Slice || actual.Kind() != reflect.Slice {
		fmt.Fprintf(
			b,
			"expected %#v, got %#v",
			expected.Interface(),
			actual.Interface(),
		)
		return
	}
	n := max(expected.Len(), actual.Len())
	for i := 0; i < n; i++ {
		switch {
		case i >= expected.Len():
			fmt.Fprintf(
				b,
				"%s[%d]: expected <missing>, got %s\n",
				prefix,
				i,
				shortValue(actual.Index(i)),
			)
		case i >= actual.Len():
			fmt.Fprintf(
				b,
				"%s[%d]: expected %s, got <missing>\n",
				prefix,
				i,
				shortValue(expected.Index(i)),
			)
		default:
			ev, av := expected.Index(i), actual.Index(i)
			if reflect.DeepEqual(ev.Interface(), av.Interface()) {
				continue
			}
			fmt.Fprintf(
				b,
				"%s[%d]: expected %s, got %s\n",
				prefix,
				i,
				shortValue(ev),
				shortValue(av),
			)
		}
	}
}

// diffMap добавляет поэлементное сравнение карт по ключам обоих значений.
func diffMap(b *strings.Builder, expected, actual reflect.Value) {
	diffMapAt(b, "{}", expected, actual)
}

// diffMapAt сравнивает карты под общим префиксом пути.
func diffMapAt(
	b *strings.Builder,
	prefix string,
	expected,
	actual reflect.Value,
) {
	if expected.Kind() != reflect.Map || actual.Kind() != reflect.Map {
		fmt.Fprintf(
			b,
			"expected %#v, got %#v",
			expected.Interface(),
			actual.Interface(),
		)
		return
	}
	seen := map[any]bool{}
	for _, k := range expected.MapKeys() {
		seen[k.Interface()] = true
		ev, av := expected.MapIndex(k), actual.MapIndex(k)
		if !av.IsValid() {
			fmt.Fprintf(
				b,
				"%s[%s]: expected %s, got <missing>\n",
				prefix,
				shortValue(k),
				shortValue(ev),
			)
			continue
		}
		if reflect.DeepEqual(ev.Interface(), av.Interface()) {
			continue
		}
		fmt.Fprintf(
			b,
			"%s[%s]: expected %s, got %s\n",
			prefix,
			shortValue(k),
			shortValue(ev),
			shortValue(av),
		)
	}
	for _, k := range actual.MapKeys() {
		if seen[k.Interface()] {
			continue
		}
		fmt.Fprintf(
			b,
			"%s[%s]: expected <missing>, got %s\n",
			prefix,
			shortValue(k),
			shortValue(actual.MapIndex(k)),
		)
	}
}

// isByteSlice сообщает, является ли значение именно []byte
// (не именованным типом на его основе).
func isByteSlice(v any) bool {
	_, ok := v.([]byte)
	return ok
}

// isByteValue сообщает, является ли значение reflect-представления []byte.
func isByteValue(v reflect.Value) bool {
	return v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8
}

// toBytes приводит значение к []byte, если это []byte, иначе возвращает nil.
func toBytes(v any) []byte {
	b, _ := v.([]byte)
	return b
}

// shortBytes усекает байтовый срез для вывода в diff.
func shortBytes(b []byte) string {
	const maxBytes = 64
	if len(b) <= maxBytes {
		return fmt.Sprintf("%#v", b)
	}
	return fmt.Sprintf("%#v...(%d bytes)", b[:maxBytes], len(b))
}

// shortValue форматирует значение через %#v с усечением до diffMaxElement.
func shortValue(v reflect.Value) string {
	if !v.IsValid() {
		return "<invalid>"
	}
	return truncate(fmt.Sprintf("%#v", v.Interface()), diffMaxElement)
}

// truncate усекает строку до limit рун с многоточием.
func truncate(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit]) + "..."
}

// truncateDiff ограничивает суммарный вывод diff по строкам и символам.
func truncateDiff(s string) string {
	s = strings.TrimRight(s, "\n")
	lines := strings.Split(s, "\n")
	if len(lines) > diffMaxLines {
		lines = append(lines[:diffMaxLines], fmt.Sprintf(
			"... (%d more lines)",
			len(lines)-diffMaxLines,
		))
	}
	out := strings.Join(lines, "\n")
	return truncate(out, diffMaxRunes)
}
