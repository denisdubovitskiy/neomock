package format

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSource проверяет форматирование gofmt-некорректного кода.
func TestSource(t *testing.T) {
	t.Parallel()

	// arrange
	src := []byte("package mocks\n\nfunc  f( ) int {\nreturn 1}\n")

	// act
	got, err := Source(src)

	// assert
	require.NoError(t, err)
	assert.Equal(
		t,
		"package mocks\n\nfunc f() int {\n\treturn 1\n}\n",
		string(got),
	)
}

// TestSource_Error сообщает ошибку синтаксиса, а не молчит.
func TestSource_Error(t *testing.T) {
	t.Parallel()

	// act
	_, err := Source([]byte("package mocks\nfunc {\n"))

	// assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "format:")
}

// TestWrite проверяет все три статуса записи и атомарность (§5).
func TestWrite(t *testing.T) {
	t.Parallel()

	t.Run("new file written", func(t *testing.T) {
		t.Parallel()

		// arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "mock.go")

		// act
		st, err := Write(path, []byte("package mocks\n"), false)

		// assert
		require.NoError(t, err)
		assert.Equal(t, Written, st)
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "package mocks\n", string(data))
	})

	t.Run("identical bytes leave mtime untouched", func(t *testing.T) {
		t.Parallel()

		// arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "mock.go")
		_, err := Write(path, []byte("package mocks\n"), true)
		require.NoError(t, err)
		info1, err := os.Stat(path)
		require.NoError(t, err)

		// act: повторная запись тех же байтов с force=true
		st, err := Write(path, []byte("package mocks\n"), true)

		// assert
		require.NoError(t, err)
		assert.Equal(t, Unchanged, st)
		info2, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(
			t,
			info1.ModTime(),
			info2.ModTime(),
			"mtime должен сохраниться — нет пересборки Go",
		)
	})

	t.Run("changed bytes with force rewrite", func(t *testing.T) {
		t.Parallel()

		// arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "mock.go")
		_, err := Write(path, []byte("package mocks\n"), true)
		require.NoError(t, err)

		// act
		st, err := Write(path, []byte("package mocks\n// v2\n"), true)

		// assert
		require.NoError(t, err)
		assert.Equal(t, Written, st)
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "package mocks\n// v2\n", string(data))
	})

	t.Run("changed bytes without force preserve manual edits", func(t *testing.T) {
		t.Parallel()

		// arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "mock.go")
		_, err := Write(path, []byte("package mocks\n"), true)
		require.NoError(t, err)

		// act
		st, err := Write(path, []byte("package mocks\n// regenerated\n"), false)

		// assert
		require.NoError(t, err)
		assert.Equal(t, SkippedManual, st)
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "package mocks\n", string(data), "ручная правка не затёрта")
	})

	t.Run("existing mode preserved on rewrite", func(t *testing.T) {
		t.Parallel()

		// arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "mock.go")
		_, err := Write(path, []byte("package mocks\n"), true)
		require.NoError(t, err)
		require.NoError(t, os.Chmod(path, 0o600))

		// act
		st, err := Write(path, []byte("package mocks\n// v2\n"), true)

		// assert
		require.NoError(t, err)
		assert.Equal(t, Written, st)
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	})

	t.Run("no temp files left behind", func(t *testing.T) {
		t.Parallel()

		// arrange
		dir := t.TempDir()
		path := filepath.Join(dir, "mock.go")
		_, err := Write(path, []byte("package mocks\n"), true)
		require.NoError(t, err)
		_, err = Write(path, []byte("package mocks\n// v2\n"), true)
		require.NoError(t, err)

		// act
		entries, err := os.ReadDir(dir)

		// assert
		require.NoError(t, err)
		assert.Len(t, entries, 1, "в каталоге только сам mock.go — tmp-файлы убраны")
	})
}
