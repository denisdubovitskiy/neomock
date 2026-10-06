// Package genmocks — behavior-тесты сгенерированных моков (dogfooding):
// сквозной срез M1 из плана (When/Then, wildcard Context, кардинальность,
// Run-fallback, Verify, сообщения §3.2.4, гонки). Файлы *_mock.go
// генерируются TestRender_Golden из internal/emitter (NEOMOCK_UPDATE_GOLDEN=1).
package genmocks

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	neomock "github.com/denisdubovitskiy/neomock"
)

// ctxKey — типизированный ключ контекста (SA1029: не built-in string).
type ctxKey string

// fakeT — ручной Tester: собирает сообщения и очистки, Fatalf паникует
// (фаза настройки и strict-режим).
type fakeT struct {
	mu       sync.Mutex
	errs     []string
	logs     []string
	cleanups []func()
}

func (f *fakeT) Helper() {}

func (f *fakeT) Cleanup(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleanups = append(f.cleanups, fn)
}

func (f *fakeT) Errorf(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errs = append(f.errs, fmt.Sprintf(format, args...))
}

func (f *fakeT) Fatalf(format string, args ...any) {
	panic("fatalf: " + fmt.Sprintf(format, args...))
}

func (f *fakeT) Logf(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs = append(f.logs, fmt.Sprintf(format, args...))
}

func (f *fakeT) runCleanups() {
	f.mu.Lock()
	fns := append([]func(){}, f.cleanups...)
	f.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

func (f *fakeT) errorCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.errs)
}

func (f *fakeT) allErrors() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.errs, "\n")
}

// TestWhenThen — базовое ожидание: точные аргументы отдают заданные результаты.
func TestWhenThen(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	m := NewMockServer(&fakeT{})
	m.WhenGet(ctx, "k1").Then("v1", nil)

	// act
	v, err := m.Get(ctx, "k1")

	// assert
	require.NoError(t, err)
	assert.Equal(t, "v1", v)
}

// TestContextWildcard — neomock.Context матчит любой не-nil контекст,
// nil-контекст — mismatch (DESIGN 3.2.2).
func TestContextWildcard(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	m := NewMockServer(&fakeT{})
	m.WhenGet(neomock.Context, "k2").Then("v2", nil)

	// act
	var nilCtx context.Context // намеренно nil: wildcard не должен матчить nil
	v, err := m.Get(context.WithValue(ctx, ctxKey("k"), "v"), "k2")
	_, errNil := m.Get(nilCtx, "k2")

	// assert
	require.NoError(t, err)
	assert.Equal(t, "v2", v)
	assert.Error(
		t,
		errNil,
		"Get(nil, k2) должен вернуть ошибку при wildcard-ожидании",
	)
}

// TestTimesSequence — два When с Times(1) работают как очередь.
func TestTimesSequence(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	f := &fakeT{}
	m := NewMockServer(f)
	m.WhenGet(ctx, "k").Then("v1", nil).Times(1)
	m.WhenGet(ctx, "k").Then("v2", nil).Times(1)

	// act
	v1, _ := m.Get(ctx, "k")
	v2, _ := m.Get(ctx, "k")

	// assert
	assert.Equal(t, "v1", v1)
	assert.Equal(t, "v2", v2)
	assert.Zero(t, f.errorCount(), "unexpected errors: %s", f.allErrors())
}

// TestOnceAlias — Once эквивалентен Times(1).
func TestOnceAlias(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	m := NewMockServer(&fakeT{})
	m.WhenGet(ctx, "k").Once().Then("v", nil)

	// act
	v, _ := m.Get(ctx, "k")

	// assert
	assert.Equal(t, "v", v)
}

// TestTimesZeroIsSetupError — Times(0) — ошибка настройки с подсказкой Never().
func TestTimesZeroIsSetupError(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	m := NewMockServer(&fakeT{})

	defer func() {
		// assert
		r := recover()
		require.NotNil(t, r, "Times(0) must Fatalf with hint")
		assert.Contains(t, fmt.Sprint(r), "use Never()")
	}()

	// act
	m.WhenGet(ctx, "k").Then("v", nil).Times(0)
}

// TestOptionalAndNeverSilent — неисполненный Optional и Never-запрет
// без вызова молчат в Verify.
func TestOptionalAndNeverSilent(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	f := &fakeT{}
	m := NewMockServer(f)
	m.WhenGet(ctx, "opt").Optional().Then("v", nil)
	m.WhenPut(ctx, "gone").Never()

	// act
	f.runCleanups()

	// assert
	assert.Zero(
		t,
		f.errorCount(),
		"optional unmet / never uncall must be silent: %s",
		f.allErrors(),
	)
}

// TestNeverViolation — вызов под Never возвращает ErrUnexpectedCall и
// помеченное сообщение.
func TestNeverViolation(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	f := &fakeT{}
	m := NewMockServer(f)
	m.WhenPut(ctx, "k").Never()

	// act
	err := m.Put(ctx, "k")
	f.runCleanups()

	// assert
	require.Error(t, err)
	assert.ErrorIs(t, err, neomock.ErrUnexpectedCall)
	assert.Contains(t, f.allErrors(), "explicitly forbidden by Never()")
}

// TestNeverAllViolation — NeverGet запрещает любой вызов метода:
// ноль результатов, ErrUnexpectedCall и сообщение с NeverGet().
func TestNeverAllViolation(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	f := &fakeT{}
	m := NewMockServer(f)
	m.NeverGet()

	// act
	v, err := m.Get(ctx, "any")
	f.runCleanups()

	// assert
	assert.Equal(t, "", v)
	require.Error(t, err)
	assert.ErrorIs(t, err, neomock.ErrUnexpectedCall)
	assert.Contains(t, f.allErrors(), "explicitly forbidden by NeverGet()")
}

// TestNeverAllWinsOverWhen — NeverGet запрещает метод независимо от
// порядка регистрации относительно When.
func TestNeverAllWinsOverWhen(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	f := &fakeT{}
	m := NewMockServer(f)
	m.WhenGet(ctx, "k").Then("v", nil).Optional()
	m.NeverGet()

	// act
	_, err := m.Get(ctx, "k")
	f.runCleanups()

	// assert
	require.Error(t, err)
	assert.ErrorIs(t, err, neomock.ErrUnexpectedCall)
	assert.Contains(t, f.allErrors(), "explicitly forbidden by NeverGet()")
	assert.Equal(t, 1, f.errorCount(), "only the forbidden-call report, no missing-call noise")
}

// TestNeverAllSilentWhenNotCalled — неисполненный NeverXxx не даёт
// сообщений в Verify.
func TestNeverAllSilentWhenNotCalled(t *testing.T) {
	t.Parallel()

	// arrange
	f := &fakeT{}
	m := NewMockServer(f)
	m.NeverGet()

	// act
	f.runCleanups()

	// assert
	assert.Zero(t, f.errorCount(), "untriggered NeverXxx must be silent: %s", f.allErrors())
}

// TestNeverAllDuplicate — повторный NeverGet: предупреждение в Logf,
// запрет остаётся от первой регистрации.
func TestNeverAllDuplicate(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	f := &fakeT{}
	m := NewMockServer(f)
	m.NeverGet()

	// act
	m.NeverGet()
	_, err := m.Get(ctx, "k")

	// assert
	require.Error(t, err)
	assert.Contains(t, f.allErrors(), "explicitly forbidden by NeverGet()")
	require.Len(t, f.logs, 1)
	assert.Contains(t, f.logs[0], "duplicate NeverGet")
}

// TestNeverAllChaining — NeverXxx возвращает мок, правила цепляются.
func TestNeverAllChaining(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	f := &fakeT{}
	m := NewMockServer(f)

	// act
	m.NeverGet().NeverPut()
	errPut := m.Put(ctx, "k")
	_, errGet := m.Get(ctx, "k")

	// assert
	require.Error(t, errPut)
	require.Error(t, errGet)
	assert.Contains(t, f.allErrors(), "explicitly forbidden by NeverPut()")
	assert.Contains(t, f.allErrors(), "explicitly forbidden by NeverGet()")
}

// TestNeverAllStrict — в строгом режиме нарушение останавливает тест.
func TestNeverAllStrict(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	f := &fakeT{}
	m := NewMockServer(f, neomock.WithStrictCalls())
	m.NeverGet()

	// act & assert
	require.Panics(t, func() { _, _ = m.Get(ctx, "k") })
}

// TestRunFallback — When первыми, Run — fallback для несовпавших аргументов.
func TestRunFallback(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	m := NewMockServer(&fakeT{})
	m.WhenGet(ctx, "k").Then("v", nil)
	m.RunGet(func(ctx context.Context, id string) (string, error) {
		return "run:" + id, nil
	})

	// act
	vMatched, _ := m.Get(ctx, "k")
	vFallback, _ := m.Get(ctx, "other")

	// assert
	assert.Equal(t, "v", vMatched, "When must win over Run")
	assert.Equal(t, "run:other", vFallback, "Run fallback failed")
}

// TestRunDuplicate — второй Run — ошибка с обоими origin.
func TestRunDuplicate(t *testing.T) {
	t.Parallel()

	// arrange
	m := NewMockServer(&fakeT{})
	m.RunGet(func(ctx context.Context, id string) (string, error) { return "", nil })

	defer func() {
		// assert
		r := recover()
		require.NotNil(t, r, "second Run must Fatalf with both origins")
		assert.Contains(t, fmt.Sprint(r), "already set at")
	}()

	// act
	m.RunGet(func(ctx context.Context, id string) (string, error) { return "", nil })
}

// TestUnexpectedCall — вызов без ожидания: одна soft-ошибка с маркером.
func TestUnexpectedCall(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	f := &fakeT{}
	m := NewMockServer(f)

	// act
	_, err := m.Get(ctx, "nope")

	// assert
	require.Error(t, err)
	assert.ErrorIs(t, err, neomock.ErrUnexpectedCall)
	require.Equal(
		t,
		1,
		f.errorCount(),
		"exactly one Errorf expected: %s",
		f.allErrors(),
	)
	assert.Contains(t, f.errs[0], "no matching When expectation")
}

// TestArgumentMismatch — несовпадение аргумента: репорт с позицией и diff.
func TestArgumentMismatch(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	f := &fakeT{}
	m := NewMockServer(f)
	m.WhenGet(ctx, "k1").Then("v", nil)

	// act
	_, _ = m.Get(ctx, "k9")

	// assert
	require.NotZero(t, f.errorCount(), "mismatch must report")
	assert.Contains(t, f.errs[0], "argument mismatch")
	assert.Contains(t, f.errs[0], `arg[1] (id): expected "k1"`)
}

// TestMissingCallVerify — неисполненное ожидание: один репорт в Verify,
// повторный NeomockVerify идемпотентен.
func TestMissingCallVerify(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	f := &fakeT{}
	m := NewMockServer(f)
	m.WhenGet(ctx, "k1").Then("v", nil)

	// act
	f.runCleanups()
	m.NeomockVerify()

	// assert
	require.Equal(
		t,
		1,
		f.errorCount(),
		"one missing-call report expected: %s",
		f.allErrors(),
	)
	assert.Contains(t, f.errs[0], "missing call(s)")
	assert.Equal(
		t,
		1,
		f.errorCount(),
		"verify must be idempotent: %s",
		f.allErrors(),
	)
}

// TestWhenWithoutThen — When без Then: ошибка и в момент вызова, и в Verify.
func TestWhenWithoutThen(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	f := &fakeT{}
	m := NewMockServer(f)
	m.WhenGet(ctx, "k")

	// act
	_, _ = m.Get(ctx, "k")
	f.runCleanups()

	// assert
	require.Equal(
		t,
		2,
		f.errorCount(),
		"expected 2 reports (call + verify): %s",
		f.allErrors(),
	)
	assert.Contains(t, f.errs[0], "has no Then")
}

// TestHistory — история вызовов: параметры и счётчик.
func TestHistory(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	m := NewMockServer(&fakeT{})
	m.WhenGet(neomock.Context, "k").Then("v", nil)

	// act
	_, _ = m.Get(ctx, "k")

	// assert
	calls := m.GetCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "k", calls[0].Id)
	assert.Equal(t, 1, m.GetCallsCount())
}

// TestHistoryClonesByteSlice — мутация буфера после вызова не меняет
// историю (клон первого уровня).
func TestHistoryClonesByteSlice(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	m := NewMockStore(&fakeT{})
	m.WhenPut(ctx, "k", nil).Optional()
	buf := []byte{1, 2, 3}

	// act
	_ = m.Put(ctx, "k", buf)
	buf[0] = 99

	// assert
	calls := m.PutCalls()
	require.Len(t, calls, 1)
	require.NotNil(t, calls[0].Val)
	assert.Equal(t, byte(1), calls[0].Val[0], "history must hold a clone")
}

// TestVoidNotify — void-метод: When без Then считается исполненным.
func TestVoidNotify(t *testing.T) {
	t.Parallel()

	// arrange
	f := &fakeT{}
	m := NewMockStore(f)
	m.WhenNotify("hello")

	// act
	m.Notify("hello")
	m.Notify("hello") // кардинальность по умолчанию — 1 или более
	f.runCleanups()

	// assert
	assert.Zero(
		t,
		f.errorCount(),
		"void When is complete, verify must be silent: %s",
		f.allErrors(),
	)
}

// TestVariadicPut — variadic-хвост: равенство длин + поэлементно, пустой
// матчит пустой, разная длина — mismatch.
func TestVariadicPut(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	f := &fakeT{}
	m := NewMockStore(f)
	m.WhenPut(ctx, "k", []byte("v"), "a", "b").Then(nil)
	m.WhenPut(ctx, "k2", nil).Then(nil)

	// act
	errMatch := m.Put(ctx, "k", []byte("v"), "a", "b")
	errEmpty := m.Put(ctx, "k2", nil)
	_ = m.Put(ctx, "k2", nil, "extra")

	// assert
	assert.NoError(t, errMatch, "variadic match failed")
	assert.NoError(t, errEmpty, "empty variadic must match empty")
	assert.NotZero(t, f.errorCount(), "variadic length mismatch must report")
}

// TestCountWithoutContext — метод без контекста: точное совпадение аргумента.
func TestCountWithoutContext(t *testing.T) {
	t.Parallel()

	// arrange
	m := NewMockStore(&fakeT{})
	m.WhenCount("user:").Then(42)

	// act
	got := m.Count("user:")

	// assert
	assert.Equal(t, 42, got)
}

// TestDuplicateShadowWarning — дубль unbounded When: предупреждение в Logf.
func TestDuplicateShadowWarning(t *testing.T) {
	t.Parallel()

	// arrange
	f := &fakeT{}
	m := NewMockServer(f)

	// act
	m.WhenGet(neomock.Context, "k").Then("v1", nil)
	m.WhenGet(neomock.Context, "k").Then("v2", nil)

	// assert
	require.Len(t, f.logs, 1)
	assert.Contains(t, f.logs[0], "duplicate unbounded WhenGet")
}

// TestPanicWithoutConstructor — использование мока без конструктора паникует.
func TestPanicWithoutConstructor(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	defer func() {
		// assert
		r := recover()
		require.NotNil(t, r, "expected used-without-Tester panic")
		assert.Contains(t, fmt.Sprint(r), "used without Tester")
	}()

	// arrange
	var m *MockServer

	// act
	_, _ = m.Get(ctx, "k")
}

// TestPanicOnNonMockVerify — neomock.Verify на произвольном типе паникует.
func TestPanicOnNonMockVerify(t *testing.T) {
	t.Parallel()

	defer func() {
		// assert
		r := recover()
		require.NotNil(t, r, "neomock.Verify(non-mock) must panic")
		assert.Contains(t, fmt.Sprint(r), "not a neomock mock")
	}()

	// act
	neomock.Verify(struct{}{})
}

// TestStrictMode — strict-calls: несовпавший вызов — Fatalf (panic у fakeT).
func TestStrictMode(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	defer func() {
		// assert
		r := recover()
		require.NotNil(t, r, "strict mode must Fatalf")
		assert.Contains(t, fmt.Sprint(r), "unexpected call")
	}()

	// arrange
	f := &fakeT{}
	m := NewMockServer(f, neomock.WithStrictCalls())

	// act
	_, _ = m.Get(ctx, "nope")
}

// TestConcurrentCalls — параллельные вызовы и чтение истории без гонок
// (детектор -race) и с полной историей.
func TestConcurrentCalls(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	m := NewMockServer(&fakeT{})
	m.WhenGet(neomock.Context, "k").Optional().Then("v", nil)
	const (
		workers = 16
		iters   = 50
	)

	// act
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				_, _ = m.Get(ctx, "k")
				_ = len(m.GetCalls())
			}
		}()
	}
	wg.Wait()

	// assert
	assert.Equal(t, workers*iters, m.GetCallsCount())
}

// TestConcurrentSequences — конкурентные вызовы одного метода под -race.
func TestConcurrentSequences(t *testing.T) {
	t.Parallel()

	// arrange
	m := NewMockStore(&fakeT{})
	m.WhenCount("k").Optional().Then(1)
	const (
		workers = 8
		iters   = 100
	)

	// act
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				_ = m.Count("k")
			}
		}()
	}
	wg.Wait()

	// assert
	assert.Equal(t, workers*iters, m.CountCallsCount())
}

// TestGenericMock — generic-мок: типизированные When/Then и wildcard ctx.
func TestGenericMock(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	m := NewMockGeneric[string](&fakeT{})
	m.WhenGet(ctx, "a").Then("b", nil)
	m.WhenGet(neomock.Context, "w").Then("v", nil)

	// act
	got, err := m.Get(ctx, "a")
	gotWildcard, _ := m.Get(ctx, "w")

	// assert
	require.NoError(t, err)
	assert.Equal(t, "b", got)
	assert.Equal(t, "v", gotWildcard, "wildcard with generic")
}

// TestCardinalityBothOrders — порядок кардинальности не важен: до Then и
// после (DESIGN 3.2.1).
func TestCardinalityBothOrders(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	m := NewMockServer(&fakeT{})
	m.WhenGet(ctx, "k").Times(2).Then("v", nil)
	m.WhenGet(ctx, "k2").Then("v", nil).Times(2)

	// act
	var errs []error
	for i := 0; i < 2; i++ {
		_, err := m.Get(ctx, "k")
		errs = append(errs, err)
		_, err = m.Get(ctx, "k2")
		errs = append(errs, err)
	}

	// assert
	for i, err := range errs {
		assert.NoErrorf(t, err, "call %d", i)
	}
}

// TestExhaustedExpectationFallsToRun — исчерпанный When уступает Run.
func TestExhaustedExpectationFallsToRun(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	m := NewMockServer(&fakeT{})
	m.WhenGet(ctx, "k").Once().Then("v", nil)
	m.RunGet(func(ctx context.Context, id string) (string, error) {
		return "fallback", nil
	})

	// act
	vFirst, _ := m.Get(ctx, "k")
	vSecond, _ := m.Get(ctx, "k")

	// assert
	assert.Equal(t, "v", vFirst, "first call must be served by When")
	assert.Equal(t, "fallback", vSecond, "exhausted When must fall through to Run")
}

// TestNeverViolationSkipsRun — Never блокирует и When, и Run-fallback
// (DESIGN 3.2.1).
func TestNeverViolationSkipsRun(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	f := &fakeT{}
	m := NewMockServer(f)
	m.WhenPut(ctx, "k").Never()
	runCalled := false
	m.RunPut(func(ctx context.Context, v string) error {
		runCalled = true
		return nil
	})

	// act
	_ = m.Put(ctx, "k")

	// assert
	assert.False(
		t,
		runCalled,
		"Run fallback must NOT be called for Never violation",
	)
	assert.Contains(t, f.allErrors(), "explicitly forbidden by Never()")
}

// TestVerifyReportsTimesShortfall — нехватка кардинальности: shortfall-репорт.
func TestVerifyReportsTimesShortfall(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	f := &fakeT{}
	m := NewMockServer(f)
	m.WhenGet(ctx, "k").Then("v", nil).Times(2)

	// act
	_, _ = m.Get(ctx, "k")
	f.runCleanups()

	// assert
	require.Equal(
		t,
		1,
		f.errorCount(),
		"one shortfall report expected: %s",
		f.allErrors(),
	)
	assert.Contains(t, f.errs[0], "expected at least 2 call(s), got 1")
}

// TestUnexpectedMessageDumpsExpectations — unexpected-сообщение содержит
// дамп ожиданий с origin.
func TestUnexpectedMessageDumpsExpectations(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	f := &fakeT{}
	m := NewMockServer(f)
	m.WhenGet(ctx, "k1").Then("v", nil).Once()

	// act
	_, _ = m.Get(ctx, "k1")    // обслуженный вызов молчит
	_, _ = m.Get(ctx, "other") // исчерпано → unexpected с дампом

	// assert
	require.NotEmpty(t, f.errs)
	assert.Contains(t, f.errs[0], "no matching When expectation")
	assert.Contains(t, f.errs[0], "[0] WhenGet(")
	assert.Contains(t, f.errs[0], " at ")
}

// TestGetCallsInitiallyEmpty — у свежего мока история пуста.
func TestGetCallsInitiallyEmpty(t *testing.T) {
	t.Parallel()

	// arrange
	m := NewMockServer(&fakeT{})

	// act & assert
	assert.Empty(t, m.GetCalls(), "fresh mock must have empty history")
	assert.Zero(t, m.GetCallsCount())
}

// TestConcurrentMixedMethods — конкурентные вызовы разных методов и
// истории без гонок (детектор -race).
func TestConcurrentMixedMethods(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	m := NewMockStore(&fakeT{})
	m.WhenGet(neomock.Context, "k").Optional().Then("v", nil)
	m.WhenCount("c").Optional().Then(1)
	m.WhenNotify("n")
	const (
		workers = 8
		iters   = 50
	)

	// act
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				_, _ = m.Get(ctx, "k")
				_ = m.Count("c")
				m.Notify("n")
				_ = m.GetCallsCount()
				_ = m.PutCallsCount()
			}
		}()
	}
	wg.Wait()

	// assert
	assert.Equal(t, workers*iters, m.GetCallsCount())
	assert.Equal(t, workers*iters, m.CountCallsCount())
}
