package neomock

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ctxKey — типизированный ключ контекста (SA1029: не built-in string).
type ctxKey string

// errFixture — фикстурная ошибка для таблицы сравнения.
var errFixture = errors.New("a")

// TestEqual_ContextWildcard проверяет wildcard neomock.Context (DESIGN 3.2.2).
func TestEqual_ContextWildcard(t *testing.T) {
	t.Parallel()

	t.Run("matches any non-nil context", func(t *testing.T) {
		t.Parallel()

		// assert
		assert.True(t, Equal(Context, context.Background()))
		assert.True(
			t,
			Equal(Context, context.WithValue(context.Background(), ctxKey("k"), "v")),
		)
	})

	t.Run("rejects nil and non-context", func(t *testing.T) {
		t.Parallel()

		// assert
		assert.False(t, Equal(Context, nil))
		assert.False(t, Equal(Context, "not-a-context"))
	})

	t.Run("exact context values match", func(t *testing.T) {
		t.Parallel()

		// assert
		assert.True(t, Equal(context.Background(), context.Background()))
	})
}

// TestEqual_Bytes проверяет семантику []byte как в require.Equal (DESIGN 3.2.4).
func TestEqual_Bytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		expected any
		actual   any
		want     bool
	}{
		{"nil equals empty", []byte(nil), []byte{}, true},
		{"equal content", []byte("abc"), []byte("abc"), true},
		{"different content", []byte("abc"), []byte("abd"), false},
		{"different length", []byte("ab"), []byte("abc"), false},
		{"type mismatch", []byte("a"), "a", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// assert
			assert.Equal(t, tt.want, Equal(tt.expected, tt.actual))
		})
	}
}

// TestEqual_Table проверяет таблицу типов из DESIGN 3.2.4.
func TestEqual_Table(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name     string
		expected any
		actual   any
		want     bool
	}{
		{"nil equals nil", nil, nil, true},
		{"equal strings", "a", "a", true},
		{"different strings", "a", "b", false},
		{"equal ints", 1, 1, true},
		{"both funcs nil", (func())(nil), (func())(nil), true},
		{"one func nil", (func())(nil), func() {}, false},
		{"both funcs non-nil", func() {}, func() {}, false},
		{"equal time", now, now, true},
		{"different time", now, now.Add(time.Second), false},
		{"same error instance", errFixture, errFixture, true},
		{"deep-equal error instances", errFixture, errors.New("a"), true},
		{"nil slice not empty", []string(nil), []string{}, false},
		{"equal slice", []string{"a"}, []string{"a"}, true},
		{"nil map not empty", map[string]int(nil), map[string]int{}, false},
		{"equal map", map[string]int{"a": 1}, map[string]int{"a": 1}, true},
		{"equal struct", struct{ A int }{1}, struct{ A int }{1}, true},
		{"different struct", struct{ A int }{1}, struct{ A int }{2}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// assert
			assert.Equal(t, tt.want, Equal(tt.expected, tt.actual))
		})
	}
}

// TestIsAnyContext проверяет детект маркера.
func TestIsAnyContext(t *testing.T) {
	t.Parallel()

	// assert
	assert.True(t, IsAnyContext(Context))
	assert.False(
		t,
		IsAnyContext(context.Background()),
		"обычный контекст не маркер",
	)
	assert.False(t, IsAnyContext(nil))
}

// TestCaller проверяет формат file:line.
func TestCaller(t *testing.T) {
	t.Parallel()

	// act
	got := Caller(0)

	// assert
	assert.Contains(t, got, ".go:")
}

// TestDiff проверяет построчный вывод diff.
func TestDiff(t *testing.T) {
	t.Parallel()

	t.Run("empty for equal values", func(t *testing.T) {
		t.Parallel()

		// assert
		assert.Empty(t, Diff("a", "a"))
	})

	t.Run("struct by fields", func(t *testing.T) {
		t.Parallel()

		// act
		got := Diff(
			struct{ Name, City string }{"a", "Berlin"},
			struct{ Name, City string }{"a", "Munich"},
		)

		// assert
		assert.Contains(t, got, `Field .City: expected "Berlin", got "Munich"`)
	})

	t.Run("slice by elements", func(t *testing.T) {
		t.Parallel()

		// act
		got := Diff([]int{1, 2}, []int{1, 3})

		// assert
		assert.Contains(t, got, "[][1]: expected 2, got 3")
	})

	t.Run("map by keys", func(t *testing.T) {
		t.Parallel()

		// act
		got := Diff(map[string]int{"a": 1}, map[string]int{"a": 2})

		// assert
		assert.Contains(t, got, "expected 1, got 2")
	})

	t.Run("long output truncated", func(t *testing.T) {
		t.Parallel()

		// arrange
		long := make([]int, 100)
		for i := range long {
			long[i] = i
		}

		// act
		got := Diff(long, make([]int, 100))

		// assert
		lineCount := strings.Count(got, "\n") + 1
		assert.LessOrEqual(t, lineCount, diffMaxLines+1)
	})
}

// TestVerify_PanicOnNonMock проверяет панику Verify на не-моке.
func TestVerify_PanicOnNonMock(t *testing.T) {
	t.Parallel()

	// assert
	require.PanicsWithValue(t, "neomock.Verify: not a neomock mock", func() {
		Verify(struct{}{})
	})
}

// TestApplyOptions проверяет приоритет опции над базой (конфигом).
func TestApplyOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		base Settings
		opts []Option
		want bool
	}{
		{"default soft", Settings{}, nil, false},
		{"config value kept", Settings{StrictCalls: true}, nil, true},
		{"option enables strict", Settings{}, []Option{WithStrictCalls()}, true},
		{"option keeps strict", Settings{StrictCalls: true}, []Option{WithStrictCalls()}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// act
			got := ApplyOptions(tt.base, tt.opts...)

			// assert
			assert.Equal(t, tt.want, got.StrictCalls)
		})
	}
}

// TestFormatValue проверяет усечение до 500 символов.
func TestFormatValue(t *testing.T) {
	t.Parallel()

	t.Run("short value as %#v", func(t *testing.T) {
		t.Parallel()

		// assert
		assert.Equal(t, `"abc"`, FormatValue("abc"))
	})

	t.Run("long value truncated", func(t *testing.T) {
		t.Parallel()

		// arrange
		long := strings.Repeat("x", 1000)

		// act
		got := FormatValue(long)

		// assert
		assert.LessOrEqual(t, len([]rune(got)), maxFormattedValue+3)
	})
}

// TestFormatArgs проверяет сборку строки аргументов.
func TestFormatArgs(t *testing.T) {
	t.Parallel()

	// act
	got := FormatArgs([]string{"ctx", "key"}, []any{Context, "k1"})

	// assert
	assert.Contains(t, got, "ctx=")
	assert.Contains(t, got, `key="k1"`)
}

// TestFormatExpected проверяет читабельный рендер маркера Context.
func TestFormatExpected(t *testing.T) {
	t.Parallel()

	t.Run("marker rendered as neomock.Context", func(t *testing.T) {
		t.Parallel()

		// assert
		assert.Equal(t, "neomock.Context", FormatExpected(Context))
	})

	t.Run("regular values as FormatValue", func(t *testing.T) {
		t.Parallel()

		// assert
		assert.Equal(t, `"k1"`, FormatExpected("k1"))
	})
}
