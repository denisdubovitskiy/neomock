package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseGenerateArgs_ModeA — полный набор флагов режима А (§4.3).
func TestParseGenerateArgs_ModeA(t *testing.T) {
	t.Parallel()

	// act
	f, err := ParseGenerateArgs([]string{
		"--interface", "Store", "--interface", "Cache",
		"--out", "./mocks/store_mock.go",
		"--pkgname", "mocks",
		"--structname", "MockStore",
		"--source", "test",
		"--inpackage",
		"--strict-calls",
		"--history=false",
		"--build-tags", "integration,slow",
		"--boilerplate", "lic.txt",
	})

	// assert
	require.NoError(t, err)
	assert.Equal(t, []string{"Store", "Cache"}, f.Interfaces)
	assert.Equal(t, "./mocks/store_mock.go", f.Out)
	assert.Equal(t, "mocks", f.Pkgname)
	assert.Equal(t, "MockStore", f.Structname)
	assert.Equal(t, "test", f.Source)
	assert.True(t, f.InPackage)
	assert.True(t, f.StrictCalls)
	require.NotNil(t, f.History)
	assert.False(t, *f.History)
	assert.Equal(t, "integration,slow", f.BuildTags)
	assert.Equal(t, "lic.txt", f.Boilerplate)
}

// TestParseGenerateArgs_Equals — форма --flag=value.
func TestParseGenerateArgs_Equals(t *testing.T) {
	t.Parallel()

	// act
	f, err := ParseGenerateArgs(
		[]string{"--interface=Store", "--out=./x.go", "--inpackage=true"},
	)

	// assert
	require.NoError(t, err)
	assert.Equal(t, []string{"Store"}, f.Interfaces)
	assert.Equal(t, "./x.go", f.Out)
	assert.True(t, f.InPackage)
}

// TestParseGenerateArgs_UnknownFlag — неизвестный флаг — ошибка (§4.3).
func TestParseGenerateArgs_UnknownFlag(t *testing.T) {
	t.Parallel()
	_, err := ParseGenerateArgs([]string{"--nope", "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown flag")
}

// TestParseGenerateArgs_MissingValue — флаг без значения — ошибка.
func TestParseGenerateArgs_MissingValue(t *testing.T) {
	t.Parallel()
	_, err := ParseGenerateArgs([]string{"--out"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "требует значение")
}

// TestParseGenerateArgs_BadSource — --source валидируется (§4.3).
func TestParseGenerateArgs_BadSource(t *testing.T) {
	t.Parallel()
	_, err := ParseGenerateArgs([]string{"--source", "maybe"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "go|test|all")
}

// TestGenFlags_SettingsLayer — флаги режима А становятся слоем слияния.
func TestGenFlags_SettingsLayer(t *testing.T) {
	t.Parallel()

	// arrange
	f, err := ParseGenerateArgs(
		[]string{
			"--interface",
			"S",
			"--out",
			"x.go",
			"--pkgname",
			"m",
			"--strict-calls",
		},
	)
	require.NoError(t, err)

	// act
	s := f.SettingsLayer()

	// assert
	require.NotNil(t, s.Pkgname)
	assert.Equal(t, "m", *s.Pkgname)
	require.NotNil(t, s.StrictCalls)
	assert.True(t, *s.StrictCalls)
	assert.Nil(t, s.Source, "не заданный флаг не создаёт значение")
}

// TestParseGenerateArgs_GenerateLine — --generate-line=false попадает в слой.
func TestParseGenerateArgs_GenerateLine(t *testing.T) {
	t.Parallel()

	// act
	f, err := ParseGenerateArgs([]string{"--interface", "S", "--out", "x.go", "--generate-line=false"})
	require.NoError(t, err)

	// assert
	require.NotNil(t, f.GenerateLine)
	assert.False(t, *f.GenerateLine)
	s := f.SettingsLayer()
	require.NotNil(t, s.GenerateLine)
	assert.False(t, *s.GenerateLine)
}
