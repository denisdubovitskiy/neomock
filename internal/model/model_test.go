package model

import (
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fn помогает создать *types.Func для интерфейса.
func fn(pkg *types.Package, name string, sig *types.Signature) *types.Func {
	return types.NewFunc(0, pkg, name, sig)
}

// ctxPkg — фиктивный пакет "context" с типом Context.
var ctxPkg = types.NewPackage("context", "context")

// ctxType — types.Type, идентичный context.Context (для isContext).
var ctxType = types.NewNamed(
	types.NewTypeName(0, ctxPkg, "Context", nil),
	types.NewStruct(nil, nil),
	nil,
)

// TestFromNamed_Basic строит интерфейс и проверяет поля.
func TestFromNamed_Basic(t *testing.T) {
	t.Parallel()

	// arrange
	pkg := types.NewPackage("example.com/svc", "svc")
	sig := types.NewSignatureType(nil, nil, nil,
		types.NewTuple(types.NewVar(0, pkg, "ctx", ctxType)),
		types.NewTuple(types.NewVar(0, pkg, "", types.Typ[types.String])),
		false,
	)
	iface := types.NewInterfaceType(
		[]*types.Func{fn(pkg, "Get", sig)},
		nil,
	).Complete()
	named := types.NewNamed(types.NewTypeName(0, pkg, "Repo", nil), iface, nil)

	// act
	got, err := FromNamed(named)

	// assert
	require.NoError(t, err)
	assert.Equal(t, "Repo", got.Name)
	assert.True(t, got.Exported)
	assert.Equal(t, "example.com/svc", got.Pkg.Path)
	assert.Len(t, got.Methods, 1)
	assert.Equal(t, "Get", got.Methods[0].Name)
	assert.True(t, got.Methods[0].Params[0].IsContext)
}

// TestFromNamed_NonInterface rejects non-interface types.
func TestFromNamed_NonInterface(t *testing.T) {
	t.Parallel()

	// arrange
	pkg := types.NewPackage("example.com/svc", "svc")
	named := types.NewNamed(
		types.NewTypeName(0, pkg, "Repo", nil),
		types.NewStruct(nil, nil),
		nil,
	)

	// act
	_, err := FromNamed(named)

	// assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not an interface")
}

// TestFromNamed_EmbeddedFlattening разворачивает встраивание и записывает closure.
func TestFromNamed_EmbeddedFlattening(t *testing.T) {
	t.Parallel()

	// arrange
	pkg := types.NewPackage("example.com/svc", "svc")
	sig := types.NewSignatureType(nil, nil, nil, nil, nil, false)
	base := types.NewInterfaceType(
		[]*types.Func{fn(pkg, "Base", sig)},
		nil,
	).Complete()
	baseNamed := types.NewNamed(types.NewTypeName(0, pkg, "Base", nil), base, nil)
	root := types.NewInterfaceType(
		[]*types.Func{},
		[]types.Type{baseNamed},
	).Complete()
	named := types.NewNamed(types.NewTypeName(0, pkg, "Root", nil), root, nil)

	// act
	got, err := FromNamed(named)

	// assert
	require.NoError(t, err)
	assert.Len(t, got.Methods, 1, "метод Base плоский")
	assert.Len(t, got.Closure, 2, "корень + Base в замыкании")
}

// TestFromNamed_DiamondDedup не дублирует метод из двух путей встраивания.
func TestFromNamed_DiamondDedup(t *testing.T) {
	t.Parallel()

	pkg := types.NewPackage("example.com/svc", "svc")
	sig := types.NewSignatureType(nil, nil, nil, nil, nil, false)
	common := types.NewInterfaceType(
		[]*types.Func{fn(pkg, "Common", sig)},
		nil,
	).Complete()
	commonNamed := types.NewNamed(
		types.NewTypeName(0, pkg, "Common", nil),
		common,
		nil,
	)

	a := types.NewInterfaceType(
		[]*types.Func{},
		[]types.Type{commonNamed},
	).Complete()
	b := types.NewInterfaceType(
		[]*types.Func{},
		[]types.Type{commonNamed},
	).Complete()
	root := types.NewInterfaceType([]*types.Func{}, []types.Type{a, b}).Complete()
	named := types.NewNamed(types.NewTypeName(0, pkg, "Root", nil), root, nil)

	// act
	got, err := FromNamed(named)

	// assert
	require.NoError(t, err)
	assert.Len(t, got.Methods, 1, "Common из двух путей — один метод")
}

// TestFromNamed_Conflict reports error for diamond with different signatures.
func TestFromNamed_Conflict(t *testing.T) {
	t.Parallel()

	pkg := types.NewPackage("example.com/svc", "svc")
	sigA := types.NewSignatureType(nil, nil, nil,
		types.NewTuple(types.NewVar(0, pkg, "", types.Typ[types.String])), nil, false)
	sigB := types.NewSignatureType(nil, nil, nil,
		types.NewTuple(types.NewVar(0, pkg, "", types.Typ[types.Int])), nil, false)
	a := types.NewInterfaceType([]*types.Func{fn(pkg, "M", sigA)}, nil).Complete()
	b := types.NewInterfaceType([]*types.Func{fn(pkg, "M", sigB)}, nil).Complete()
	root := types.NewInterfaceType([]*types.Func{}, []types.Type{a, b}).Complete()
	named := types.NewNamed(types.NewTypeName(0, pkg, "Root", nil), root, nil)

	// act
	_, err := FromNamed(named)

	// assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflict method M")
}

// TestFromNamed_MethodOrder проверяет детерминированный порядок методов:
// сортировка по имени (go/types отдаёт ExplicitMethod отсортированным).
// Детерминированный вывод обязателен для skip-write-if-equal (§5).
func TestFromNamed_MethodOrder(t *testing.T) {
	t.Parallel()

	// arrange
	pkg := types.NewPackage("example.com/svc", "svc")
	mk := func(name string) *types.Func {
		sig := types.NewSignatureType(
			nil,
			nil,
			nil,
			types.NewTuple(),
			types.NewTuple(),
			false,
		)
		return fn(pkg, name, sig)
	}
	iface := types.NewInterfaceType(
		[]*types.Func{mk("Zebra"), mk("Alpha"), mk("Middle")},
		nil,
	).Complete()
	named := types.NewNamed(types.NewTypeName(0, pkg, "Ordered", nil), iface, nil)

	// act
	got, err := FromNamed(named)
	require.NoError(t, err)

	// assert: порядок декларации, не алфавитный и не map-случайный
	names := make([]string, 0, len(got.Methods))
	for _, m := range got.Methods {
		names = append(names, m.Name)
	}
	assert.Equal(t, []string{"Alpha", "Middle", "Zebra"}, names)
}

// TestFromNamed_NamedSliceAndMapFlags проверяет, что именованные типы на
// основе slice/map получают флаги IsSlice/IsMap по underlying (DESIGN 3.2.5
// п.4: история клонируется slices.Clone/maps.Clone, они принимают ~[]E/~map).
func TestFromNamed_NamedSliceAndMapFlags(t *testing.T) {
	t.Parallel()

	// arrange
	pkg := types.NewPackage("example.com/svc", "svc")
	ids := types.NewNamed(
		types.NewTypeName(0, pkg, "IDs", nil),
		types.NewSlice(types.Typ[types.String]),
		nil,
	)
	lookup := types.NewNamed(
		types.NewTypeName(0, pkg, "Lookup", nil),
		types.NewMap(types.Typ[types.String], types.Typ[types.Int]),
		nil,
	)
	fn := types.NewNamed(
		types.NewTypeName(0, pkg, "Fn", nil),
		types.NewSignatureType(
			nil,
			nil,
			nil,
			types.NewTuple(),
			types.NewTuple(),
			false,
		),
		nil,
	)

	mk := func(name string, paramType types.Type) *types.Func {
		sig := types.NewSignatureType(nil, nil, nil,
			types.NewTuple(types.NewVar(0, pkg, "v", paramType)),
			types.NewTuple(),
			false,
		)
		return types.NewFunc(0, pkg, name, sig)
	}
	iface := types.NewInterfaceType(
		[]*types.Func{mk("TakeIDs", ids), mk("TakeLookup", lookup), mk("TakeFn", fn)},
		nil,
	).Complete()
	named := types.NewNamed(types.NewTypeName(0, pkg, "Repo", nil), iface, nil)

	// act
	got, err := FromNamed(named)
	require.NoError(t, err)

	// assert
	byName := map[string]*Method{}
	for _, m := range got.Methods {
		byName[m.Name] = m
	}
	assert.True(
		t,
		byName["TakeIDs"].Params[0].IsSlice,
		"именованный слайс — IsSlice",
	)
	assert.False(t, byName["TakeIDs"].Params[0].IsMap)
	assert.True(
		t,
		byName["TakeLookup"].Params[0].IsMap,
		"именованная карта — IsMap",
	)
	assert.False(t, byName["TakeFn"].Params[0].IsSlice, "func-тип не клонируется")
	assert.False(t, byName["TakeFn"].Params[0].IsMap)
}

// TestFromNamed_ErrorParamNoPanic — регрессия: `error` в ПАРАМЕТРАХ метода
// (universe-тип, Pkg() == nil) не должен паниковать в isContext; флаг
// IsContext при этом false.
func TestFromNamed_ErrorParamNoPanic(t *testing.T) {
	t.Parallel()

	// arrange
	pkg := types.NewPackage("example.com/svc", "svc")
	sig := types.NewSignatureType(nil, nil, nil,
		types.NewTuple(
			types.NewVar(0, pkg, "ctx", ctxType),
			types.NewVar(0, pkg, "err", types.Universe.Lookup("error").Type()),
		),
		types.NewTuple(),
		false,
	)
	iface := types.NewInterfaceType(
		[]*types.Func{fn(pkg, "Log", sig)},
		nil,
	).Complete()
	named := types.NewNamed(types.NewTypeName(0, pkg, "Log", nil), iface, nil)

	// act
	got, err := FromNamed(named)

	// assert
	require.NoError(t, err)
	require.Len(t, got.Methods, 1)
	assert.False(
		t,
		got.Methods[0].Params[1].IsContext,
		"error — не context.Context",
	)
	assert.True(t, got.Methods[0].Params[0].IsContext)
}
