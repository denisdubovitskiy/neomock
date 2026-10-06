---
title: Разработка
---

# Разработка: сборка, тесты, golden-моки

How-to для контрибьютора репозитория. Инструменты: Go 1.26+ (tool-директива),
poetry — только для сборки документации.

## Команды

```sh
# Полная проверка (обязательна перед завершением любой задачи)
go build ./... && go vet ./... && go test -race ./...

# Один пакет
go test -race ./internal/emitter

# Линтер (golangci-lint в Docker, кэш — в volume; настройки — .golangci.yml)
make lint

# CI локально через act (нужен Docker; make act-list — список джобов)
make act

# Документация: dev-сервер на http://localhost:8000
make docs-serve

# Документация: строгая сборка (предупреждения = ошибки)
make docs-build
```

`make docs-*` использует локальное poetry-окружение (`pyproject.toml`,
виртуальное окружение `.venv` в корне репозитория); poetry установит
зависимости при первом запуске.

## Сгенерированные моки и golden-тесты

Моки `internal/emitter/genmocks/*_mock.go` — **не редактировать руками**:
это эталоны, сгенерированные эмиттером (dogfooding) для 8 фикстур
(Server, Store, Rich, ReaderWriter, Diamond, Greet, IntStore, Generic).
Рядом лежат behavior-тесты, которые гоняют DSL под `-race`.

После изменения шаблона или эмиттера:

```sh
NEOMOCK_UPDATE_GOLDEN=1 go test ./internal/emitter   # перегенерировать эталоны
go test -race ./...                                  # убедиться, что behavior-тесты зелёные
```

Golden-тест сверяет вывод эмиттера с эталонами байт-в-байт: любой
недетерминизм в генерации сразу станет падающим тестом.

## Границы репозитория

- `internal/emitter/genmocks/*_mock.go` — только через регенерацию golden.
- Рантайм в корне модуля (пакет `neomock`) и весь сгенерированный код —
  **zero-dependency**: только stdlib и `neomock`. `testify` допустим только
  в тестах генератора (`internal/...`).
- `sandbox/` — локальные исследовательские копии mockery/minimock/gomock
  (отдельные go-модули, не коммитятся): не собирать, не рефакторить.

## Внутренние пакеты

| Пакет | Роль |
|---|---|
| корень (`neomock`) | рантайм моков: `Tester`, `Context`, `Equal`, `Diff`, `Verify`, опции |
| `internal/loader` | загрузка интерфейсов через `go/packages` |
| `internal/model` | внутреннее представление интерфейса; разворачивание встраивания |
| `internal/registry` | учёт импортов, алиасы, рендер типов с квалификацией |
| `internal/names` | имена сущностей генерата; fail-fast проверки коллизий |
| `internal/emitter` | сборка данных + `text/template` (`mock.tmpl`); golden-моки в `genmocks` |
| `internal/format` | `go/format.Source`, атомарная запись, skip-if-equal |
| `internal/config`, `cmd/neomock` | конфигурация (YAML/TOML, слои, go:generate) и CLI |

## Договорённости процесса

- **`CHANGELOG.md`** — все заметные изменения фиксируются в секции
  `Unreleased` в том же PR, что и изменение.
- Обоснования решений — в [архитектуре](architecture.md).
- Стиль тестов генератора: testify, комментарии arrange/act/assert,
  race-детектор обязателен.
- Сообщения в сгенерированном коде — на английском (expected/got).
- Вывод генератора обязан быть байт-детерминированным: не добавляйте обход
  map в порядок генерации (методы — по имени, импорты — по пути).

## Дальше

- [Архитектура](architecture.md) — почему конвейер и рантайм устроены так.
- [Обзор](index.md) — статус компонентов.
