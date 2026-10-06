# Changelog

Все заметные изменения проекта документируются в этом файле.
Формат — [Keep a Changelog](https://keepachangelog.com/ru/1.1.0/),
проект следует [Semantic Versioning](https://semver.org/lang/ru/).

## [0.1.0] - 2026-10-07

Первый релиз.

### Added

- **Рантайм `neomock`** (zero-dependency, только stdlib): `Tester`,
  `Context` (wildcard для контекста), `Equal` (семантика `require.Equal`),
  `Diff`, `Verify`, `ErrUnexpectedCall`, опции конструктора
  (`WithStrictCalls`). Мягкий режим ошибок по умолчанию (безопасен в
  горутинах), строгий — opt-in.
- **Типизированный DSL сгенерированного мока**: `WhenXxx(...).Then(...)`
  с конкретными типами аргументов, кардинальность `Times/Once/Optional/Never`,
  запрет метода целиком `NeverXxx()` (без привязки к аргументам и истории),
  `RunXxx` со сигнатурой метода для сложных случаев, история вызовов
  `XxxCalls`/`XxxCallsCount` с клонированием слайсов/мапов, авто-`Verify`
  через `t.Cleanup`.
- **Генератор**: загрузка интерфейсов через `go/packages` (свой модуль,
  stdlib, внешние зависимости, `vendor/`, `go.work`), generics, приватные
  интерфейсы, `source: go|test|all`, директива `//neomock:generate=false`.
- **Конфигурация** YAML/TOML (одна схема): селекторы пакетов
  (`all`/`include`/`exclude`/`interfaces`), шаблоны имён, слои приоритета с
  трассировкой источника, `go:generate`-режимы А (флаги, без конфига) и Б
  (короткая строка), ключи `inpackage`, `history`, `strict-calls`,
  `force-write`, `verify-build`, `generate-line` и др.
- **CLI** (`go tool neomock`): `init`, `generate` (идемпотентная запись,
  `--clean` для orphaned-моков), `check` для CI (unified diff, SARIF 2.1.0,
  exit 1 при устаревших моках), `explain` (значение + источник каждой
  настройки), `migrate` с mockery, `version`.
- **Детерминированный вывод**: байт-в-байт стабильная генерация,
  skip-write при совпадении (mtime не меняется), атомарная запись.
- Документация `docs/` (MkDocs Material), демо-модуль `example/`
  (YAML/TOML-конфиги, все сценарии размещения моков), golden-моки
  (dogfooding) и сквозные e2e-тесты.
- CI (GitHub Actions): линтер `golangci-lint` и тесты под `-race` на Go 1.26;
  локально — `make lint` (Docker) и `make act`.
