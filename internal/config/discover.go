package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Discover находит файл конфигурации (§4.1, §4.6):
//   - explicit (--config / NEOMOCK_CONFIG) — обязан существовать;
//   - иначе поиск .neomock.yml/.yaml/.toml в dir; два файла рядом — ошибка.
//
// Пустой результат — конфиг-режим без файла (go:generate, §4.3).
func Discover(explicit, dir string) (path string, format string, err error) {
	if explicit != "" {
		if _, err := os.Stat(explicit); err != nil {
			return "", "", fmt.Errorf("config: файл конфигурации %s: %w", explicit, err)
		}
		f, err := formatOf(explicit)
		if err != nil {
			return "", "", err
		}
		return explicit, f, nil
	}
	// Поиск от dir вверх: go:generate выполняется в каталоге пакета,
	// а файл конфига лежит в корне модуля (§4.1).
	cur := dir
	for {
		var found []string
		for _, name := range FileNames {
			p := filepath.Join(cur, name)
			if _, err := os.Stat(p); err == nil {
				found = append(found, p)
			}
		}
		switch len(found) {
		case 0:
		case 1:
			f, err := formatOf(found[0])
			if err != nil {
				return "", "", err
			}
			return found[0], f, nil
		default:
			names := make([]string, 0, len(found))
			for _, p := range found {
				names = append(names, filepath.Base(p))
			}
			return "", "", fmt.Errorf(
				"config: найдено несколько файлов конфигурации рядом: %s — оставьте один (§4.1)",
				strings.Join(names, ", "),
			)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", "", nil
		}
		cur = parent
	}
}

// LoadFile читает и разбирает файл конфигурации.
func LoadFile(path, format string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	return Parse(data, format)
}

func formatOf(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yml", ".yaml":
		return "yaml", nil
	case ".toml":
		return "toml", nil
	default:
		return "", fmt.Errorf(
			"config: %s: расширение должно быть .yml/.yaml/.toml",
			path,
		)
	}
}
