package genmocks

// Behavior-тесты экзотических типов (фикстура shapes): именованные слайсы,
// struct с Diff, func/chan-семантика Equal, клонирование истории, variadic.

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/denisdubovitskiy/neomock/internal/loader/testdata/shapes"
)

// TestRich_QueryExoticTypes — экзотические параметры/результаты: именованные
// слайсы, struct, map, []byte, три результата.
func TestRich_QueryExoticTypes(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	m := NewMockRich(&fakeT{})
	in := shapes.Filter{Name: "n", Limit: 1}
	m.WhenQuery(ctx, in, shapes.IDs{"a"}, nil, nil, nil, []byte("x")).
		Then(
			map[string]int{"a": 1},
			[]shapes.UserID{7},
			nil,
		)

	// act
	gotMap, gotIDs, err := m.Query(
		ctx,
		shapes.Filter{Name: "n", Limit: 1},
		shapes.IDs{"a"},
		nil,
		nil,
		nil,
		[]byte("x"),
	)

	// assert
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"a": 1}, gotMap)
	assert.Equal(t, []shapes.UserID{7}, gotIDs)
}

// TestRich_StructMismatchDiffByFields — несовпадение struct матчится по полям
// и сообщает имя поля через Diff.
func TestRich_StructMismatchDiffByFields(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	f := &fakeT{}
	m := NewMockRich(f)
	m.WhenQuery(
		ctx,
		shapes.Filter{Name: "n", Limit: 1},
		nil,
		nil,
		nil,
		nil,
		nil,
	).Optional()

	// act
	_, _, _ = m.Query(
		ctx,
		shapes.Filter{Name: "n", Limit: 2},
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	// assert
	require.NotZero(t, f.errorCount(), "struct mismatch must report")
	assert.Contains(t, f.allErrors(), "argument mismatch")
	assert.Contains(
		t,
		f.allErrors(),
		"Field .Limit: expected 1, got 2",
		"Diff must go first and name the field",
	)
}

// TestRich_ByteSliceNilEqualsEmpty — семантика require.Equal: nil []byte
// равен пустому (DESIGN 3.2.4).
func TestRich_ByteSliceNilEqualsEmpty(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	m := NewMockRich(&fakeT{})
	m.WhenQuery(
		ctx,
		shapes.Filter{},
		nil,
		nil,
		nil,
		nil,
		[]byte{},
	).Then(nil, nil, nil)

	// act
	_, _, err := m.Query(ctx, shapes.Filter{}, nil, nil, nil, nil, nil)

	// assert
	assert.NoError(t, err, "nil []byte must match empty []byte")
}

// TestRich_MapNilNotEqualsEmpty — nil map НЕ равен пустому (DeepEqual-семантика).
func TestRich_MapNilNotEqualsEmpty(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	f := &fakeT{}
	m := NewMockRich(f)
	m.WhenMap(ctx, map[string]int{}).Optional().Then(nil)

	// act
	_ = m.Map(ctx, nil)

	// assert
	assert.NotZero(
		t,
		f.errorCount(),
		"nil map must NOT match empty map (DeepEqual-семантика)",
	)
}

// TestRich_HistoryClonesNamedSliceAndMap — мутация аргументов после вызова
// не меняет историю (клон map и именованного slice).
func TestRich_HistoryClonesNamedSliceAndMap(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	m := NewMockRich(&fakeT{})
	m.WhenMap(ctx, map[string]int{"a": 1}).Optional().Then(nil)
	m.WhenQuery(
		ctx,
		shapes.Filter{},
		shapes.IDs{"a"},
		nil,
		nil,
		nil,
		nil,
	).Optional().Then(nil, nil, nil)
	in := map[string]int{"a": 1}
	ids := shapes.IDs{"a"}

	// act
	_ = m.Map(ctx, in)
	in["added"] = 2 // мутация после вызова не должна менять историю
	_, _, _ = m.Query(ctx, shapes.Filter{}, ids, nil, nil, nil, nil)
	ids[0] = "changed"

	// assert
	require.Len(t, m.MapCalls(), 1)
	assert.Zero(
		t,
		m.MapCalls()[0].In["added"],
		"map history must be cloned: %v",
		m.MapCalls()[0].In,
	)
	require.Len(t, m.QueryCalls(), 1)
	assert.Equal(
		t,
		"a",
		string(m.QueryCalls()[0].Ids[0]),
		"named slice history must be cloned",
	)
}

// TestRich_FuncParamOnlyNilMatches — func-параметры: nil матчится только с
// nil; реальную функцию можно проверить только через Run (§3.2.4).
func TestRich_FuncParamOnlyNilMatches(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	f := &fakeT{}
	m := NewMockRich(f)
	m.WhenQuery(ctx, shapes.Filter{}, nil, nil, nil, nil, nil).Optional()

	// act: не-nil функции не равны никогда — сравнение по значению отвергнуто
	_, _, _ = m.Query(
		ctx,
		shapes.Filter{},
		nil,
		func(x int) error { return nil },
		nil,
		nil,
		nil,
	)
	// nil vs nil совпадают (ожидание без Then — результат не проверяем)
	_, _, _ = m.Query(ctx, shapes.Filter{}, nil, nil, nil, nil, nil)

	// assert
	assert.NotZero(t, f.errorCount(), "non-nil fn must not match nil expectation")

	// проверка не-nil fn — через Run: Run получает реальную функцию и может её вызвать
	m2 := NewMockRich(&fakeT{})
	var called bool
	m2.RunQuery(func(
		ctx context.Context,
		fl shapes.Filter,
		ids shapes.IDs,
		fn shapes.Fn,
		ch chan int,
		p *shapes.Filter,
		b []byte,
	) (map[string]int, []shapes.UserID, error) {
		require.NotNil(t, fn, "Run must receive the real fn")
		require.NoError(t, fn(1))
		called = true
		return nil, nil, nil
	})

	// act
	_, _, _ = m2.Query(
		ctx,
		shapes.Filter{},
		nil,
		func(x int) error { return nil },
		nil,
		nil,
		nil,
	)

	// assert
	assert.True(t, called, "Run-side fn was not invoked")
}

// TestRich_ChannelIdentity — канал совпадает по идентичности, другой канал — нет.
func TestRich_ChannelIdentity(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	m := NewMockRich(&fakeT{})
	ch := make(chan int, 1)
	m.WhenQuery(
		ctx,
		shapes.Filter{},
		nil,
		nil,
		ch,
		nil,
		nil,
	).Optional().Then(nil, nil, nil)

	// act
	_, _, errSame := m.Query(ctx, shapes.Filter{}, nil, nil, ch, nil, nil)
	_, _, errOther := m.Query(
		ctx,
		shapes.Filter{},
		nil,
		nil,
		make(chan int, 1),
		nil,
		nil,
	)

	// assert
	assert.NoError(t, errSame, "same channel must match (DeepEqual по ссылке)")
	assert.Error(t, errOther, "different channel must not match")
}

// TestRich_OnlyErr — единственный error-результат: Then(nil) и Then(err).
func TestRich_OnlyErr(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// arrange
	m := NewMockRich(&fakeT{})
	m.WhenOnlyErr(ctx).Then(nil).Once()
	want := errors.New("boom")
	m.WhenOnlyErr(ctx).Then(want)

	// act
	errNil := m.OnlyErr(ctx)
	errWant := m.OnlyErr(ctx)

	// assert
	assert.NoError(t, errNil, "Then(nil) must return nil")
	assert.ErrorIs(t, errWant, want, "Then(err) must return it")
}

// TestRich_NoArgsAlwaysMatch — метод без аргументов: match всегда true.
func TestRich_NoArgsAlwaysMatch(t *testing.T) {
	t.Parallel()

	// arrange
	m := NewMockRich(&fakeT{})
	m.WhenNoArgs().Then(5, nil)

	// act
	got, err := m.NoArgs()

	// assert
	require.NoError(t, err)
	assert.Equal(t, 5, got)
}

// TestRich_Variadic — variadic: точное совпадение, пустой матчит пустой
// (len 0 == len 0), разная длина — mismatch с указанием позиции.
func TestRich_Variadic(t *testing.T) {
	t.Parallel()

	// arrange
	f := &fakeT{}
	m := NewMockRich(f)
	m.WhenVariadic("p", "a", "b").Then(3)
	m.WhenVariadic("q").Then(0)
	m.WhenVariadic("r").Optional().Then(0)

	// act
	gotMatch := m.Variadic("p", "a", "b")
	gotEmpty := m.Variadic("q")
	_ = m.Variadic("r", "extra")

	// assert
	assert.Equal(t, 3, gotMatch)
	assert.Zero(t, gotEmpty, "empty variadic must match")
	// репорт против первого неисчерпанного кандидата — When("p","a","b"),
	// отсюда expected len 2
	assert.Contains(t, f.allErrors(), "arg[1] (parts): expected len 2, got len 1")
}
