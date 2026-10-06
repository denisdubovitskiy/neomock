// Package neomock — рантайм сгенерированных моков.
//
// Пакет содержит минимум, необходимый сгенерированному коду: интерфейс Tester,
// wildcard neomock.Context для контекстов, сравнение Equal (семантика,
// совместимая с require.Equal), построчный Diff, Caller, sentinel
// ErrUnexpectedCall, опции конструктора и Verify.
// Внешних зависимостей нет — только стандартная библиотека.
package neomock

import (
	"context"
	"errors"
)

// Tester — минимальный интерфейс, совместимый с *testing.T.
type Tester interface {
	// Helper помечает вызывающую функцию как вспомогательную,
	// чтобы сообщения указывали на тест, а не на мок.
	Helper()
	// Cleanup регистрирует очистку, выполняемую после теста (для Verify).
	Cleanup(func())
	// Errorf фиксирует ошибку, не останавливая тест (мягкий режим вызовов).
	Errorf(format string, args ...any)
	// Fatalf останавливает тестовую горутину (строгий режим и фаза настройки).
	Fatalf(format string, args ...any)
	// Logf пишет в журнал без пометки ошибки (предупреждение о дубле When,
	// DESIGN 3.2.1). *testing.T удовлетворяет интерфейсу.
	Logf(format string, args ...any)
}

// Context — сторожевой маркер «любой контекст, не матчить».
// Использование: m.WhenGet(neomock.Context, "k").Then("v", nil).
// Работает только на прямой позиции параметра с типом context.Context;
// внутри struct-аргументов не детектится (см. DESIGN 3.2.2).
var Context context.Context = anyContext{context.Background()}

// anyContext — unexported-тип маркера: делегирует методы context.Background(),
// чтобы валидироваться как обычный контекст, но опознаваться через IsAnyContext.
type anyContext struct{ context.Context }

// IsAnyContext сообщает, является ли expected маркером neomock.Context.
func IsAnyContext(expected any) bool {
	_, ok := expected.(anyContext)
	return ok
}

// ErrUnexpectedCall — sentinel для мягкого режима: если последний результат
// метода — error, при неожиданном вызове возвращается ошибка, оборачивающая
// его (fmt.Errorf("...: %w", ErrUnexpectedCall)), а не nil.
// Проверяется через errors.Is(err, neomock.ErrUnexpectedCall).
var ErrUnexpectedCall = errors.New("neomock: unexpected call")

// Option — функциональная опция конструктора мока.
type Option func(*options)

// options — состояние, заполняемое опциями конструктора.
type options struct {
	// strictCalls включает строгий режим вызовов (Fatalf вместо Errorf).
	strictCalls bool
}

// Settings — применённые опции в формате, доступном генерату.
// Генерат вызывает ApplyOptions в конструкторе мока.
type Settings struct {
	// StrictCalls итоговый режим ошибок вызовов: true — строгий (Fatalf).
	StrictCalls bool
}

// ApplyOptions применяет опции к базовым настройкам (база — значения из
// конфигурации генерации) и возвращает итог. Приоритет: опция в коде выше
// конфига (см. DESIGN 3.2.5).
func ApplyOptions(base Settings, opts ...Option) Settings {
	o := options{strictCalls: base.StrictCalls}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return Settings{StrictCalls: o.strictCalls}
}

// WithStrictCalls включает строгий режим (Fatalf вместо Errorf на вызовы).
// Приоритет над strict-calls из конфигурации. Только для одногортинных тестов:
// из воркера Fatalf запрещён рантаймом Go.
func WithStrictCalls() Option {
	return func(o *options) { o.strictCalls = true }
}

// verifier — скрытый интерфейс верификации мока. Метод экспортирован
// (NeomockVerify): unexported-метод не подошёл бы кросс-пакетно — assertion
// скрытого интерфейса всегда давал бы false (см. DESIGN 3.2.6 п.1).
type verifier interface {
	// NeomockVerify проверяет кардинальность ожиданий всех методов мока.
	NeomockVerify()
}

// When без Then сообщает через Errorf. Конструктор мока регистрирует
// Verify в t.Cleanup автоматически; вручную — neomock.Verify(m).
// Паникует, если m не является моком neomock.
func Verify(m any) {
	v, ok := m.(verifier)
	if !ok {
		panic("neomock.Verify: not a neomock mock")
	}
	v.NeomockVerify()
}
