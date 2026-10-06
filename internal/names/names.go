// Package names вычисляет имена вспомогательных сущностей сгенерированного
// мока и проверяет коллизии с методами мокаемого интерфейса (fail-fast,
// см. DESIGN 3.2.6).
package names

import (
	"fmt"
	"go/token"
	"strings"
	"unicode"
)

// Reserved — имена, зарезервированные за кардинальностью *Call-типа:
// если мокаемый интерфейс объявляет такой метод, генерация должна упасть.
var Reserved = []string{"Times", "Then", "Once", "Optional", "Never"}

// Service — служебные имена мока: коллизия с методом интерфейса — ошибка.
var Service = []string{"NeomockVerify"}

// History — имена истории вызовов: коллизия — ошибка с подсказкой history:false.
var History = []string{"GetCalls", "GetCallsCount"}

// mockStructFields — зарезервированные имена полей структуры мока;
// коллизия имени поля-холдера с ними разрешается суффиксом.
var mockStructFields = map[string]bool{
	"t": true, "strict": true, "verifyOnce": true, "mu": true,
}

// ParamName возвращает рабочее имя параметра: исходное, если оно валидно
// (не пустое и не `_`), иначе arg{i}.
func ParamName(i int, name string) string {
	if name == "" || name == "_" {
		return fmt.Sprintf("arg%d", i)
	}
	return name
}

// Export возвращает имя с заглавной буквы — для полей Params-структуры.
func Export(name string) string {
	if name == "" {
		return "V"
	}
	r := []rune(name)
	return string(unicode.ToUpper(r[0])) + string(r[1:])
}

// Field возвращает имя поля Params-структуры с дедупликацией:
// повторное имя после Export получает суффикс _1, _2, ...
func Field(taken map[string]bool, name string) string {
	base := Export(name)
	if !taken[base] {
		taken[base] = true
		return base
	}
	for i := 1; ; i++ {
		c := fmt.Sprintf("%s_%d", base, i)
		if !taken[c] {
			taken[c] = true
			return c
		}
	}
}

// Holder возвращает имя холдера (поля-структуры состояния метода):
// MockStore + Get → mockStoreGet; mockfoo + Get → mockfooGet.
func Holder(structName, method string) string {
	base := strings.TrimPrefix(strings.TrimPrefix(structName, "Mock"), "mock")
	return "mock" + base + method
}

// HolderField возвращает имя поля-холдера в структуре мока:
// метод Get → get; коллизии с зарезервированными полями и ключевыми
// словами Go (Map → map) — суффикс _1.
func HolderField(method string) string {
	f := lowerFirst(method)
	if mockStructFields[f] || token.IsKeyword(f) {
		return f + "_1"
	}
	return f
}

// lowerFirst понижает первую букву: Get → get.
func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToLower(string(r[0])) + string(r[1:])
}

// CallType возвращает имя экспортированного *Call-типа метода.
func CallType(structName, method string) string {
	return structName + method + "Call"
}

// ParamsType возвращает имя Params-структуры истории вызовов.
func ParamsType(structName, method string) string {
	return structName + method + "Params"
}

// When возвращает имя билдера ожидания.
func When(method string) string { return "When" + method }

// Run возвращает имя установщика fallback-обработчика.
func Run(method string) string { return "Run" + method }

// Never возвращает имя метода запрета вызовов метода на уровне мока.
func Never(method string) string { return "Never" + method }

// MethodSet — контракт для Validate: имена методов мокаемого интерфейса.
type MethodSet interface {
	// MethodNames возвращает имена всех методов интерфейса.
	MethodNames() []string
}

// Validate проверяет коллизии служебных имён мока с методами интерфейса
// (fail-fast, DESIGN 3.2.6): зарезервированные имена кардинальности,
// NeomockVerify, история (с подсказкой history:false), When<M>/Run<M>.
// history — генерируются ли методы истории.
func Validate(ms MethodSet, history bool) error {
	for _, m := range ms.MethodNames() {
		for _, r := range Reserved {
			if m == r {
				return fmt.Errorf(
					"interface method %q conflicts with reserved call-builder name %q: rename the interface method (DESIGN 3.2.6)",
					m,
					r,
				)
			}
		}
		for _, s := range Service {
			if m == s {
				return fmt.Errorf(
					"interface method %q conflicts with service method %q: rename the interface method (DESIGN 3.2.6)",
					m,
					s,
				)
			}
		}
		if history {
			for _, h := range History {
				if m == h {
					return fmt.Errorf(
						"interface method %q conflicts with call history method %q: set history: false to disable history (DESIGN 3.2.6)",
						m,
						h,
					)
				}
			}
		}
		for _, other := range ms.MethodNames() {
			if When(other) == m {
				return fmt.Errorf(
					"interface method %q conflicts with generated builder %q: rename the interface method (DESIGN 3.2.6)",
					m,
					When(other),
				)
			}
			if Run(other) == m {
				return fmt.Errorf(
					"interface method %q conflicts with generated runner %q: rename the interface method (DESIGN 3.2.6)",
					m,
					Run(other),
				)
			}
			if Never(other) == m {
				return fmt.Errorf(
					"interface method %q conflicts with generated never-rule %q: rename the interface method (DESIGN 3.2.6)",
					m,
					Never(other),
				)
			}
		}
	}
	return nil
}
