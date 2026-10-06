package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/denisdubovitskiy/neomock/internal/config"
)

const initYAML = `# neomock — конфигурация генерации моков.
# Полная схема и правила: DESIGN.md §4.2 (https://github.com/denisdubovitskiy/neomock)
version: 1

defaults:
  dir: "{{.InterfaceDir}}/mocks"
  filename: "{{.InterfaceSnake}}_mock.go"
  pkgname: mocks
  structname: "{{.Mock}}{{.InterfaceName}}"   # MockStore / mockrefresher (приватный)
  source: go            # go | test | all
  inpackage: false
  strict-calls: false   # true = Fatalf на вызовах (только одногортинные тесты)
  history: true         # false отключает XxxCalls/XxxCallsCount и запись истории
  # template-data:
  #   boilerplate-file: ""   # файл с шапкой (лицензией)

packages:
  # Пример: все интерфейсы пакета
  # "example.com/proj/internal/store":
  #   all: true
  #   config:
  #     dir: "{{.InterfaceDir}}/mocks"
  #
  # Пример: точечные интерфейсы + приватный inpackage
  # "example.com/proj/internal/cache":
  #   interfaces:
  #     Cache:
  #       config:
  #         structname: "MockCache"
  #     privateCache:
  #       config:
  #         inpackage: true
  #         dir: "{{.InterfaceDir}}"
  #         pkgname: cache
  #         filename: "private_cache_mock.go"
  #
  # Пример: стандартная библиотека — dir внутрь проекта обязателен
  # "io":
  #   interfaces:
  #     Reader:
  #       config:
  #         dir: "./internal/testmocks"
  #         pkgname: testmocks
  #         filename: "reader_mock.go"
`

const initTOML = `# neomock — конфигурация генерации моков.
# Полная схема и правила: DESIGN.md §4.2 (https://github.com/denisdubovitskiy/neomock)
version = 1

[defaults]
dir = "{{.InterfaceDir}}/mocks"
filename = "{{.InterfaceSnake}}_mock.go"
pkgname = "mocks"
structname = "{{.Mock}}{{.InterfaceName}}"   # MockStore / mockrefresher (приватный)
source = "go"          # go | test | all
inpackage = false
strict-calls = false   # true = Fatalf на вызовах (только одногортинные тесты)
history = true         # false отключает XxxCalls/XxxCallsCount и запись истории

# Пример:
# [packages."example.com/proj/internal/store"]
# all = true
`

// cmdInit — создание файла конфигурации (§5): init [--format yaml|toml] [--path].
func cmdInit(args []string) int {
	format := "yaml"
	path := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--format", "-format":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "neomock: --format требует yaml|toml")
				return 2
			}
			i++
			format = args[i]
		case "--path", "-path":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "neomock: --path требует путь к файлу")
				return 2
			}
			i++
			path = args[i]
		default:
			fmt.Fprintf(os.Stderr, "neomock: init: неизвестный аргумент %q\n", args[i])
			return 2
		}
	}
	if format != "yaml" && format != "toml" {
		fmt.Fprintf(
			os.Stderr,
			"neomock: --format: допустимы yaml|toml, получено %q\n",
			format,
		)
		return 2
	}
	if path == "" {
		if format == "toml" {
			path = ".neomock.toml"
		} else {
			path = ".neomock.yml"
		}
	}
	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(
			os.Stderr,
			"neomock: файл %s уже существует — не перезаписываю\n",
			path,
		)
		return 1
	}
	content := initYAML
	if format == "toml" {
		content = initTOML
	}
	if err := os.MkdirAll(
		filepath.Dir(path),
		0o755,
	); err != nil && filepath.Dir(path) != "." {
		return exitError(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return exitError(err)
	}
	// Проверка: сгенерированный скелет обязан проходить валидацию схемы.
	if _, err := config.Parse([]byte(content), format); err != nil {
		fmt.Fprintf(
			os.Stderr,
			"neomock: внутренний ошибка — скелет конфига невалиден: %v\n",
			err,
		)
		return 1
	}
	fmt.Printf("created %s\n", path)
	return 0
}
