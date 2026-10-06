package loader

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoad_Basic загружает простой интерфейс.
func TestLoad_Basic(t *testing.T) {
	t.Parallel()

	// act
	got, err := Load(
		Config{
			PackagePath:   testdataPkg,
			InterfaceName: "Server",
		},
	)

	// assert
	require.NoError(t, err)
	assert.Equal(t, "Server", got.Name)
	assert.True(t, got.Exported)
	assert.Len(t, got.Methods, 2)
	assert.Equal(t, "Get", got.Methods[0].Name)
	assert.True(t, got.Methods[0].Params[0].IsContext)
	assert.Equal(t, "Put", got.Methods[1].Name)
}

// TestLoad_UnexportedMethodsAllowedWithoutInPackage проверяет правило §3.4:
// экспортированный интерфейс с unexported-методами загружается без inpackage —
// эмиттер автовстраивает исходный интерфейс в мок (gRPC-стиль).
func TestLoad_UnexportedMethodsAllowedWithoutInPackage(t *testing.T) {
	t.Parallel()

	// act
	got, err := Load(
		Config{
			PackagePath:   testdataPkg,
			InterfaceName: "Private",
		},
	)

	// assert
	require.NoError(t, err)
	assert.True(t, got.HasUnexportedMethods())
}

// TestLoad_UnexportedInterfaceNeedsInPackage — unexported-имя интерфейса
// недоступно из чужого пакета: мок без inpackage невозможен (§3.3).
func TestLoad_UnexportedInterfaceNeedsInPackage(t *testing.T) {
	t.Parallel()

	// act
	_, err := Load(
		Config{
			PackagePath:   "github.com/denisdubovitskiy/neomock/internal/loader/testdata/embedded",
			InterfaceName: "secret",
		},
	)

	// assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "неэкспортирован")
}

// TestLoad_BrokenConflict — конфликт встраивания не компилируется в Go:
// loader обязан сообщить ошибку типов пакета, а не паниковать.
func TestLoad_BrokenConflict(t *testing.T) {
	t.Parallel()

	// act
	_, err := Load(
		Config{
			PackagePath:   "github.com/denisdubovitskiy/neomock/internal/loader/testdata/broken",
			InterfaceName: "Conflict",
		},
	)

	// assert: конфликт отлавливает модель (§3.4) — оба источника в сообщении
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflict method Get")
}

// TestLoad_GenericInstantiatedEmbedding — встраивание инстанцированного
// generic-интерфейса разворачивается с подставленными аргументами типа.
func TestLoad_GenericInstantiatedEmbedding(t *testing.T) {
	t.Parallel()

	// act
	got, err := Load(
		Config{
			PackagePath:   "github.com/denisdubovitskiy/neomock/internal/loader/testdata/embedded",
			InterfaceName: "IntStore",
		},
	)

	// assert
	require.NoError(t, err)
	require.Len(t, got.Methods, 1)
	assert.Equal(t, "Get", got.Methods[0].Name)
	assert.Len(
		t,
		got.TypeParams,
		0,
		"мок не параметризуется: инстанциация уже подставлена",
	)
}

// TestLoad_NotFound reports clear error for missing type.
func TestLoad_NotFound(t *testing.T) {
	t.Parallel()

	// act
	_, err := Load(
		Config{
			PackagePath:   testdataPkg,
			InterfaceName: "NoSuch",
		},
	)

	// assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "не найден")
}

const (
	testdataPkg    = "github.com/denisdubovitskiy/neomock/internal/loader/testdata/pkg"
	testsrcPkg     = "github.com/denisdubovitskiy/neomock/internal/loader/testdata/testsrc"
	constraintsPkg = "github.com/denisdubovitskiy/neomock/internal/loader/testdata/constraints"
)

// TestLoad_SourceGo_IgnoresTestFiles — при source:go интерфейс из *_test.go
// не находится, сообщение подсказывает про source:test|all (С4).
func TestLoad_SourceGo_IgnoresTestFiles(t *testing.T) {
	t.Parallel()

	// act
	_, err := Load(
		Config{PackagePath: testsrcPkg, InterfaceName: "TestOnly", Source: SourceGo},
	)

	// assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source:test|all")
}

// TestLoad_SourceTest — интерфейс из внутреннего и внешнего тест-файла (§3.3).
func TestLoad_SourceTest(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"TestOnly", "External"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// act
			got, err := Load(
				Config{PackagePath: testsrcPkg, InterfaceName: name, Source: SourceTest},
			)

			// assert
			require.NoError(t, err)
			assert.Equal(t, name, got.Name)
		})
	}
}

// TestLoad_SourceAll — оба источника с дедупом: видны и prod, и test-интерфейсы.
func TestLoad_SourceAll(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"Prod", "TestOnly", "External"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// act
			got, err := Load(
				Config{PackagePath: testsrcPkg, InterfaceName: name, Source: SourceAll},
			)

			// assert
			require.NoError(t, err)
			assert.Equal(t, name, got.Name)
		})
	}
}

// TestLoad_GeneratedFileExcluded — интерфейс из файла с маркером neomock
// не находится (§3.3).
func TestLoad_GeneratedFileExcluded(t *testing.T) {
	t.Parallel()

	// act
	_, err := Load(
		Config{
			PackagePath:   "github.com/denisdubovitskiy/neomock/internal/loader/testdata/generated",
			InterfaceName: "Fake",
		},
	)

	// assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "не найден")
}

// TestLoad_ConstraintRejected — constraint-интерфейс при явном запросе —
// ошибка с внятным сообщением (§3.3).
func TestLoad_ConstraintRejected(t *testing.T) {
	t.Parallel()

	// act
	_, err := Load(
		Config{
			PackagePath:   constraintsPkg,
			InterfaceName: "Number",
		},
	)

	// assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "constraint")
}

// TestLoadPackage_EnumeratesInterfaces — селекция: все интерфейсы пакета,
// generated-файлы исключены, constraints помечены, директива учтена.
func TestLoadPackage_EnumeratesInterfaces(t *testing.T) {
	t.Parallel()

	// act
	info, err := LoadPackage(
		Config{PackagePath: constraintsPkg},
	)

	// assert
	require.NoError(t, err)
	assert.True(t, info.InModule)
	require.Len(t, info.Interfaces, 3)
	byName := map[string]InterfaceInfo{}
	for _, i := range info.Interfaces {
		byName[i.Name] = i
	}
	assert.True(t, byName["Number"].IsConstraint)
	assert.True(t, byName["Comparable"].IsConstraint)
	assert.False(t, byName["Usable"].IsConstraint)
}

// TestLoadPackage_GeneratedExcluded — generated-файл не попадает в селекцию.
func TestLoadPackage_GeneratedExcluded(t *testing.T) {
	t.Parallel()

	// act
	info, err := LoadPackage(
		Config{PackagePath: "github.com/denisdubovitskiy/neomock/internal/loader/testdata/generated"},
	)

	// assert
	require.NoError(t, err)
	require.Len(t, info.Interfaces, 1)
	assert.Equal(t, "Real", info.Interfaces[0].Name)
}

// TestLoadPackage_Directive — //neomock:generate=false помечает интерфейс
// исключённым (§4.5).
func TestLoadPackage_Directive(t *testing.T) {
	t.Parallel()

	// act
	info, err := LoadPackage(
		Config{PackagePath: "github.com/denisdubovitskiy/neomock/internal/loader/testdata/directive"},
	)

	// assert
	require.NoError(t, err)
	byName := map[string]InterfaceInfo{}
	for _, i := range info.Interfaces {
		byName[i.Name] = i
	}
	assert.False(t, byName["Active"].Disabled)
	assert.True(t, byName["Skipped"].Disabled)
	assert.True(t, byName["SkippedGroup"].Disabled)
}

// TestLoadPackage_SourceTest — селекция с source:test видит тест-варианты.
func TestLoadPackage_SourceTest(t *testing.T) {
	t.Parallel()

	// act
	info, err := LoadPackage(Config{PackagePath: testsrcPkg, Source: SourceAll})

	// assert
	require.NoError(t, err)
	names := make([]string, 0, len(info.Interfaces))
	for _, i := range info.Interfaces {
		names = append(names, i.Name)
	}
	assert.Equal(t, []string{"External", "Prod", "TestOnly"}, names)
}

// TestLoad_StdlibNotInModule — пакет стандартной библиотеки: загружается,
// InModule=false (placement rules, §3.3).
func TestLoad_StdlibNotInModule(t *testing.T) {
	t.Parallel()

	// act
	got, err := Load(Config{PackagePath: "io", InterfaceName: "Reader"})

	// assert
	require.NoError(t, err)
	assert.False(t, got.Pkg.InModule)
	assert.Equal(t, "Read", got.Methods[0].Name)
}
