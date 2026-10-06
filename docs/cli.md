---
title: CLI и генерация
---

# CLI и генерация

Справка по командам генератора: установка, `generate`/`check`/`explain`/
`init`/`migrate`, `go:generate`-режимы, коды возврата. Схема конфига — в
[конфигурации](configuration.md), сценарии — в [примерах](examples.md).

## Установка

Минимальная версия Go — 1.26 (tool-директива). В `go.mod` потребителя:

```
tool github.com/denisdubovitskiy/neomock/cmd/neomock
```

Далее бинарь доступен как `go tool neomock` — без `go install` и без
версий в PATH. Проверка:

```console
$ go tool neomock version
neomock devel
```

Версия берётся из версии модуля (`debug.ReadBuildInfo`): локальная сборка и
`replace` дают `devel`, релизный тег — `neomock v0.1.0`.

Рантайм `github.com/denisdubovitskiy/neomock` (пакет `neomock`) добавится в
зависимости автоматически при первой генерации — сгенерированные моки импортируют
только его и stdlib (**zero-dependency**).

## Команды

```text
neomock init      [--format yaml|toml] [--path PATH]
neomock generate  [пакеты...] [--config PATH] [--clean] [--log-level L]
neomock check     [пакеты...] [--config PATH] [--output text|sarif] [--log-level L]
neomock explain   <пакет.Интерфейс> [--config PATH]
neomock migrate   --from .mockery.yaml --to .neomock.yml
neomock version
```

### init

Создаёт `.neomock.yml` (или `.neomock.toml` при `--format toml`) со скелетом и
комментариями. Существующий файл **не перезаписывается** (exit 1). Опция
`--path` задаёт другое имя/расположение.

### generate

```console
$ go tool neomock generate
written      /home/dev/proj/internal/store/mocks/store_mock.go
unchanged    /home/dev/proj/internal/cache/mocks/cache_mock.go
```

- Без аргументов — обрабатывает все секции `packages:` конфига.
- Позиционные аргументы — фильтр пакетов (import path или паттерн `./...`):
  `go tool neomock generate ./internal/store/...`.
- Запись идемпотентна: файл переписывается только при расхождении байтов
  (mtime сохранён — пересборки Go нет). Статусы: `written`, `unchanged`,
  `skipped (manual edits preserved)` при `force-write: false`.
- `--clean` — удалить **orphaned**-файлы (моки с маркером neomock, которые не
  соответствуют ни одному сконфигурированному интерфейсу). Работает только при
  полном прогоне без фильтра пакетов — иначе удаление чужих моков из общих
  каталогов было бы потерей данных.
- После генерации для пакетов с `verify-build: true` выполняется `go build`
  пакета мока — ошибка сборки прерывает прогон.

### check

Режим для CI: генерация в память, сравнение с файлами на диске, **exit 1** при
расхождении.

```console
$ go tool neomock check
stale     /home/dev/proj/internal/store/mocks/store_mock.go
--- a/internal/store/mocks/store_mock.go
+++ b/internal/store/mocks/store_mock.go (generated)
@@ -12,6 +12,10 @@
...
```

- `missing` — файл мока отсутствует; `stale` — устарел; при чистоте —
  `all mocks are up to date` и exit 0.
- `--output sarif` — SARIF 2.1.0 (правило `neomock/stale-mock`) для встраивания
  в отчёты CI/GitHub.
- Orphaned-файлы печатаются отдельным списком (не влияют на exit-код).

### explain

Показывает итоговое значение каждой настройки интерфейса и **слой**, который её
задал — главный инструмент диагностики приоритетов:

```console
$ go tool neomock explain example.com/proj/internal/store.Store
interface: example.com/proj/internal/store.Store
  dir:           ./internal/store/mocks   (for-package example.com/proj/internal/store)
  filename:      store_mock.go            (builtins)
  pkgname:       mocks                    (builtins)
  structname:    MockStore                (builtins)
  source:        go                       (builtins)
  inpackage:     false                    (builtins)
  strict-calls:  false                    (builtins)
  history:       true                     (builtins)
  generate-line: true                     (builtins)
```

### migrate

Переносит `.mockery.yaml` (v3, подмножество `packages:`) в `.neomock.yml`:
пути, имена структур, селекторы. Тесты **не переписываются** — команда печатает
таблицу соответствий `mockery → neomock` для ручной правки:

```text
mock.On("Get", ...).Return(v, err)   →  m.WhenGet(ctx, "k").Then(v, err)
mock.Anything на позиции контекста   →  neomock.Context
EXPECT(*m).Get(...)                  →  m.WhenGet(...)
RunAndReturn(fn)                     →  m.RunGet(fn)
.Times(n) / .Once() / .Maybe()       →  .Times(n) / .Once() / .Optional()
AssertExpectations                   →  не нужен: Verify в Cleanup конструктора
```

### version

Версия из `debug.ReadBuildInfo` — работает и через `go tool`, и через
tool-директиву без ldflags.

## go:generate

!!! note "`go:generate` не обязателен"
    Писать `//go:generate` в исходниках **не требуется**. Три равноправных
    способа запустить генерацию:

    1. **только конфиг** — `go tool neomock generate` (из корня или с фильтром
       пакетов): все моки из `.neomock.yml` — рекомендуемый путь с единой
       точкой входа;
    2. **go:generate режим А** — строка с флагами, работает вообще без конфига;
    3. **go:generate режим Б** — короткая строка, детали из секции пакета
       конфига.

    Строка `//go:generate go tool neomock generate <pkg>` внутри
    сгенерированного файла — автоподсказка (её можно удалить): `go generate
    ./...` и `go tool neomock generate` дают одинаковый результат.

Два режима — вместо файла конфигурации (или поверх него) прямо в исходнике:

=== "Режим А — флаги"

    ```go
    //go:generate go tool neomock --interface Store --out ./mocks/store_mock.go --pkgname mocks
    ```

    Работает **без конфига**: пакет = каталог строки, интерфейс и файл — из
    флагов. `--out` перекрывает `dir`/`filename` конфига. Несколько
    `--interface` требуют `--out` с каталогом (с завершающим `/`).

=== "Режим Б — короткая строка"

    ```go
    //go:generate go tool neomock
    ```

    Берёт секцию пакета текущего каталога из конфига (конфиг ищется от cwd
    вверх). Конфига или секции пакета нет — ошибка с подсказкой.

Флаги режима А:

| Флаг | Аналог конфига |
|---|---|
| `--interface` (повторяется) | `interfaces:` |
| `--out` | `dir` + `filename` (перекрывает) |
| `--pkgname` | `pkgname` |
| `--structname` | `structname` |
| `--source go\|test\|all` | `source` |
| `--inpackage` | `inpackage` |
| `--strict-calls` | `strict-calls` |
| `--history=false` | `history` |
| `--generate-line=false` | `generate-line` |
| `--build-tags` | `build-tags` |
| `--boilerplate` | `template-data.boilerplate-file` |
| `--config` | явный конфиг для строки |

Формы: `--flag value`, `--flag=value`, булевы `--flag` / `--flag=false`.
Кавычки разбирает `go generate`, shell-семантики нет.

## Коды возврата

| Код | Значение |
|---|---|
| 0 | успех |
| 1 | ошибка выполнения (битый конфиг, ошибка генерации, `check` нашёл расхождения) |
| 2 | ошибка разбора аргументов (неизвестная команда/флаг) |

## Логирование

Предупреждения конвейера (исключения директивой, constraint-пропуски,
`source`/`filename` несовпадения) идут в stderr через slog; уровень —
`--log-level` или `NEOMOCK_LOG_LEVEL` (`debug|info|warn|error`).

## Дальше

- [Конфигурация](configuration.md) — схема, ключи, приоритеты.
- [Примеры](examples.md) — шесть сценариев размещения и типовые кейсы.
- [Использование](usage.md) — DSL мока в тестах.
