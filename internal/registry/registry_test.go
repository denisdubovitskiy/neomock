package registry

import (
	"go/types"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegister проверяет назначение алиасов и дедупликацию.
func TestRegister(t *testing.T) {
	t.Parallel()

	t.Run("deduplicates colliding package names", func(t *testing.T) {
		t.Parallel()

		// arrange
		r := New("")

		// act
		a1, err1 := r.Register("html/template", "template")
		a2, err2 := r.Register("text/template", "template")
		again, _ := r.Register("html/template", "template")

		// assert
		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.Equal(t, "template", a1)
		assert.Equal(t, "template0", a2)
		assert.Equal(t, a1, again, "повторная регистрация даёт тот же алиас")
	})

	t.Run("skips self package", func(t *testing.T) {
		t.Parallel()

		// arrange
		r := New("example.com/proj/mocks")

		// act
		a, err := r.Register("example.com/proj/mocks", "mocks")

		// assert
		require.NoError(t, err)
		assert.Empty(t, a, "self-пакет не импортируется")
		assert.Empty(t, r.Imports())
	})

	t.Run("empty alias for universe types", func(t *testing.T) {
		t.Parallel()

		// arrange
		r := New("")

		// act
		a, err := r.Register("", "")

		// assert
		require.NoError(t, err)
		assert.Empty(t, a)
	})
}

// TestImports_Sorted проверяет детерминированный порядок.
func TestImportsSorted(t *testing.T) {
	t.Parallel()

	// arrange
	r := New("")
	_, _ = r.Register("zpkg/z", "z")
	_, _ = r.Register("apkg/a", "a")
	_, _ = r.Register("mpkg/m", "m")

	// act
	got := r.Imports()

	// assert
	require.Len(t, got, 3)
	assert.Equal(t, "apkg/a", got[0].Path)
	assert.Equal(t, "mpkg/m", got[1].Path)
	assert.Equal(t, "zpkg/z", got[2].Path)
}

// TestTypeString проверяет рендеринг типов и регистрацию импортов.
func TestTypeString(t *testing.T) {
	t.Parallel()

	// arrange
	inner := types.NewPackage("example.com/proj/inner", "inner")
	row := types.NewNamed(
		types.NewTypeName(0, inner, "Row", nil),
		types.NewStruct(nil, nil),
		nil,
	)

	tests := []struct {
		name string
		self string
		typ  types.Type
		want string
	}{
		{"basic", "", types.Typ[types.String], "string"},
		{"slice of basic", "", types.NewSlice(types.Typ[types.Int]), "[]int"},
		{"self package unqualified", "example.com/proj/inner", row, "Row"},
		{"foreign package qualified", "", row, "inner.Row"},
		{"pointer", "", types.NewPointer(row), "*inner.Row"},
		{"map", "", types.NewMap(
			types.Typ[types.String],
			row,
		), "map[string]inner.Row"},
		{"chan", "", types.NewChan(types.SendRecv, row), "chan inner.Row"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// arrange
			r := New(tt.self)

			// act
			got, err := r.TypeString(tt.typ)

			// assert
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestTypeString_TwoPackagesSameName проверяет дедуп по имени пакета.
func TestTypeStringTwoPackagesSameName(t *testing.T) {
	t.Parallel()

	// arrange
	p1 := types.NewPackage("a/template", "template")
	p2 := types.NewPackage("b/template", "template")
	n1 := types.NewNamed(
		types.NewTypeName(0, p1, "T1", nil),
		types.NewStruct(nil, nil),
		nil,
	)
	n2 := types.NewNamed(
		types.NewTypeName(0, p2, "T2", nil),
		types.NewStruct(nil, nil),
		nil,
	)
	r := New("")

	// act
	s1, err1 := r.TypeString(n1)
	s2, err2 := r.TypeString(n2)

	// assert
	require.NoError(t, err1)
	require.NoError(t, err2)
	assert.Equal(t, "template.T1", s1)
	assert.Equal(t, "template0.T2", s2)
}

// TestTypeString_Signature проверяет рендеринг func-типов, включая variadic.
func TestTypeStringSignature(t *testing.T) {
	t.Parallel()

	// arrange
	inner := types.NewPackage("example.com/proj/inner", "inner")
	row := types.NewNamed(
		types.NewTypeName(0, inner, "Row", nil),
		types.NewStruct(nil, nil),
		nil,
	)
	// func(string, ...inner.Row) error
	sig := types.NewSignatureType(nil, nil, nil,
		types.NewTuple(
			types.NewVar(0, nil, "", types.Typ[types.String]),
			types.NewVar(0, nil, "", types.NewSlice(row)),
		),
		types.NewTuple(
			types.NewVar(0, nil, "", types.Universe.Lookup("error").Type()),
		),
		true,
	)
	r := New("")

	// act
	got, err := r.TypeString(sig)

	// assert
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(got, "func(string, ..."), "got %q", got)
	assert.True(t, strings.HasSuffix(got, ") error"), "got %q", got)
	assert.Contains(t, got, "inner.Row", "тип должен быть квалифицирован")
}

// TestTypeString_TypeArgs проверяет рекурсивный обход TypeArgs (DESIGN 3.4).
func TestTypeStringTypeArgs(t *testing.T) {
	t.Parallel()

	// arrange
	pkgs := types.NewPackage("a.com/pkgs", "pkgs")
	other := types.NewPackage("b.com/other", "other")
	v := types.NewNamed(
		types.NewTypeName(0, other, "Val", nil),
		types.NewStruct(nil, nil),
		nil,
	)
	// generic-тип pkgs.Page[V] (constraint — пустой интерфейс)
	orig := types.NewNamed(
		types.NewTypeName(0, pkgs, "Page", nil),
		types.NewStruct(nil, nil),
		nil,
	)
	orig.SetTypeParams([]*types.TypeParam{
		types.NewTypeParam(types.NewTypeName(
			0,
			pkgs,
			"V",
			nil,
		), types.NewInterfaceType(
			nil,
			nil,
		)),
	})

	// act
	page, err := types.Instantiate(nil, orig, []types.Type{v}, true)
	require.NoError(t, err)
	r := New("")
	got, err := r.TypeString(page)

	// assert
	require.NoError(t, err)
	assert.Equal(t, "pkgs.Page[other.Val]", got)
	paths := map[string]bool{}
	for _, im := range r.Imports() {
		paths[im.Path] = true
	}
	assert.True(t, paths["a.com/pkgs"], "пакет типа должен быть зарегистрирован")
	assert.True(
		t,
		paths["b.com/other"],
		"пакет аргумента типа должен быть зарегистрирован",
	)
}

// TestTypeString_InterfaceLiterals проверяет рендер анонимных интерфейсов:
// пустой → any, непустой — литералом с сигнатурами (реальные кейсы из
// mockery-конфигов: args ...interface{}, interface{ Foo() int }).
func TestTypeString_InterfaceLiterals(t *testing.T) {
	t.Parallel()

	empty := types.NewInterfaceType(nil, nil).Complete()
	nonEmpty := types.NewInterfaceType([]*types.Func{
		types.NewFunc(0, nil, "Foo", types.NewSignatureType(nil, nil, nil,
			types.NewTuple(), types.NewTuple(types.NewVar(
				0,
				nil,
				"",
				types.Typ[types.Int],
			)), false)),
	}, nil).Complete()

	tests := []struct {
		name string
		typ  types.Type
		want string
	}{
		{"empty interface renders as any", empty, "any"},
		{"any alias renders as any", types.Universe.Lookup("any").Type(), "any"},
		{"non-empty interface literal", nonEmpty, "interface{Foo() int}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// arrange
			r := New("")

			// act
			got, err := r.TypeString(tt.typ)

			// assert
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestTypeString_UniverseError проверяет universe-тип error (Pkg() == nil):
// рендер без квалификатора и без паники.
func TestTypeString_UniverseError(t *testing.T) {
	t.Parallel()

	// arrange
	r := New("")

	// act
	got, err := r.TypeString(types.Universe.Lookup("error").Type())

	// assert
	require.NoError(t, err)
	assert.Equal(t, "error", got)
	assert.Empty(t, r.Imports())
}
