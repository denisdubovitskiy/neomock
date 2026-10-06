package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDiscover_Upward — поиск файла конфига от каталога вверх (режим Б из
// подкаталога пакета, §4.1).
func TestDiscover_Upward(t *testing.T) {
	t.Parallel()

	// arrange
	root := t.TempDir()
	sub := filepath.Join(root, "internal", "pkg")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(root, ".neomock.yml"),
			[]byte("version: 1\n"),
			0o644,
		),
	)

	// act
	path, format, err := Discover("", sub)

	// assert
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, ".neomock.yml"), path)
	require.Equal(t, "yaml", format)
}

// TestDiscover_TwoFiles — два файла рядом → ошибка (§4.1).
func TestDiscover_TwoFiles(t *testing.T) {
	t.Parallel()

	// arrange
	root := t.TempDir()
	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(root, ".neomock.yml"),
			[]byte("version: 1\n"),
			0o644,
		),
	)
	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(root, ".neomock.toml"),
			[]byte("version = 1\n"),
			0o644,
		),
	)

	// act
	_, _, err := Discover("", root)

	// assert
	require.Error(t, err)
	require.Contains(t, err.Error(), "несколько файлов")
}

// TestDiscover_Explicit — явный путь обязан существовать (§4.6).
func TestDiscover_Explicit(t *testing.T) {
	t.Parallel()

	// arrange
	root := t.TempDir()
	cfg := filepath.Join(root, "custom.yml")
	require.NoError(t, os.WriteFile(cfg, []byte("version: 1\n"), 0o644))

	// act
	path, format, err := Discover(cfg, root)
	require.NoError(t, err)
	require.Equal(t, cfg, path)
	require.Equal(t, "yaml", format)

	// assert: несуществующий явный путь — ошибка
	_, _, err = Discover(filepath.Join(root, "missing.yml"), root)
	require.Error(t, err)
}

// TestDiscover_None — файла нет — пустой результат без ошибки (бесконфиг-режим).
func TestDiscover_None(t *testing.T) {
	t.Parallel()

	// act
	path, format, err := Discover("", t.TempDir())

	// assert
	require.NoError(t, err)
	require.Empty(t, path)
	require.Empty(t, format)
}
