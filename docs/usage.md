---
title: Использование
---

# DSL мока и API рантайма

Страница описывает два API: **DSL сгенерированного мока** (как настраивать
ожидания в тестах) и **пакет рантайма `neomock`** (что из него используется и
почему). Всё описанное реализовано и покрыто сквозными тестами под `-race`
(см. [development](development.md)).

## Установка и генерация

В `go.mod` потребителя — tool-директива Go 1.24+ (модуль neomock требует Go 1.26+):

```
tool github.com/denisdubovitskiy/neomock/cmd/neomock
```

Далее `go tool neomock init` создаёт скелет `.neomock.yml`, а `go tool neomock
generate` генерирует моки. Пример конфига:

```yaml
version: 1
packages:
  "example.com/proj/internal/store":
    all: true
  "io":
    interfaces:
      Reader:
        config:
          dir: "./internal/testmocks"   # внешний пакет — только внутрь модуля
          pkgname: testmocks
```

Команды: `generate [пакеты...] [--clean]` (запись только при расхождении
байтов), `check [пакеты...] [--output text|sarif]` для CI (exit 1 при
устаревших моках), `explain <пакет.Интерфейс>` (значение + источник каждой
опции), `init [--format yaml|toml] [--path PATH]` (скелет конфига),
`migrate --from .mockery.yaml --to .neomock.yml` (перенос с mockery + отчёт
по тестам), `version`.

Вместо файла — `//go:generate`-строки: короткая `//go:generate go tool
neomock` берёт секцию пакета из конфига, строка с флагами (`--interface`,
`--out`, `--pkgname`, ...) работает без конфига. Приоритет источников:
go:generate-строка сильнее файла.

!!! info
    `//go:generate` писать **не обязательно**: достаточно `go tool neomock
    generate` с конфигом — он обработает все секции `packages:`. Подробности
    и сравнение способов — в [CLI и генерации](cli.md).

## Сгенерированный мок

Для интерфейса

```go
type Store interface {
    Get(ctx context.Context, key string) (string, error)
}
```

генератор создаёт мок `MockStore` с конструктором, принимающим любой
`*testing.T`-совместимый тестер:

```go
m := mocks.NewMockStore(t)
```

Конструктор региструет автопроверку ожиданий через `t.Cleanup` — после теста
неисполненные ожидания сами упадут с понятным сообщением. Ручное создание
`&MockStore{}` без конструктора паникует при первом обращении с подсказкой
«create via NewMockStore(t)».

## When/Then: точные значения

```go
// Таблица «вход → выход»:
m.WhenGet(ctx, "k1").Then("v1", nil)
m.WhenGet(ctx, "k2").Then("v2", nil)

// Контекст не матчится — любой не-nil контекст подходит:
m.WhenGet(neomock.Context, "any").Then("fallback", nil)
```

Аргументы `When` имеют **конкретные типы** метода, а не `any`: опечатка в типе
не скомпилируется. Сравнение — `neomock.Equal`: семантика `require.Equal`
(`bytes.Equal` для `[]byte`, где `nil` равен пустому; `reflect.DeepEqual` для
остального) плюс wildcard для `neomock.Context`.

Ограничения сравнения (осознанные):

| Аргумент | Правило |
|---|---|
| `context.Context` + `neomock.Context` | wildcard: actual должен быть не-nil контекстом; `nil` — mismatch |
| `[]byte` | `bytes.Equal`, `nil` равен пустому |
| func (опции) | совпадают только оба `nil` — для проверки func-опций используйте `Run` |
| `chan` | совпадает только тот же канал (сравнение по ссылке) |
| `time.Time` | строгий `DeepEqual` (включая monotonic) — передавайте усечённые метки |
| slice/map (кроме `[]byte`) | `nil` не равен пустому; именованные типы на их основе (`type IDs []string`) — те же правила |
| struct | сравнивается целиком; wildcard контекста **внутри** полей не работает — только `Run` |

## Кардинальность

По умолчанию ожидание должно сработать **хотя бы раз** (иначе `Verify` после
теста сообщит о нехватке). Точная настройка:

```go
m.WhenGet(ctx, "k").Then("v", nil).Times(2)  // ровно 2
m.WhenGet(ctx, "k").Then("v", nil).Once()    // ровно 1 (алиас Times(1))
m.WhenGet(ctx, "k").Then("v", nil).Optional() // 0 или более
m.WhenDelete(ctx, "k").Never()                // 0 вызовов; Then не нужен
```

`Times(0)` запрещён — при настройке это `Fatalf` с подсказкой «use Never()
instead». Нарушенный `Never` при вызове
даёт ошибку с пометкой «explicitly forbidden by Never() at ...».

### Запрет метода целиком

Когда метод не должен вызываться вообще — ни с какими аргументами — удобно
не перечислять аргументы в `When`, а запретить метод целиком:

```go
m.NeverGet()                 // любой вызов Get запрещён
m.NeverGet().NeverDelete()   // правила цепляются: NeverXxx возвращает мок
```

Нарушение фиксируется в момент вызова с пометкой
«explicitly forbidden by NeverGet() at ...» — в мягком режиме это `Errorf`
и zero values (последний `error`-результат оборачивает
`neomock.ErrUnexpectedCall`), в строгом — `Fatalf`. Запрет сильнее
ожиданий: `NeverGet()` запрещает метод независимо от того, зарегистрирован
ли для него `When` и в каком порядке. Если метод так и не вызывали —
`Verify` молчит: нечего сообщать. Повторный `NeverGet()` не переписывает
первый запрет, а добавляет предупреждение в журнал.

В отличие от `When(...).Never()`, запрет на уровне метода не зависит от
аргументов и не требует истории вызовов. Если метод интерфейса называется
`NeverGet`, генерация упадёт с подсказкой переименовать — коллизии
служебных имён проверяются на этапе генерации.

Исчерпанные ожидания пропускаются при поиске, поэтому два одинаковых `When`
с `Once()` работают как **очередь** (удобно для ретраев):

```go
m.WhenGet(ctx, "k").Then("v1", nil).Once()
m.WhenGet(ctx, "k").Then("v2", nil).Once()
// первый вызов → v1, второй → v2
```

Повторное перекрытие неисчерпаемых `When` с одинаковыми аргументами (оба без
`Times`) — предупреждение в журнал «duplicate unbounded ... shadows previous»:
второе ожидание никогда не сработает.

## Run: сложные случаи

Когда точных значений мало (switch по входу, out-параметры, func-опции),
используется `Run` с полностью типизированной сигнатурой метода:

```go
m.RunGet(func(ctx context.Context, key string) (string, error) {
    switch {
    case key == "":
        return "", store.ErrNotFound
    case strings.HasPrefix(key, "tmp:"):
        return "", nil
    default:
        return "v:" + key, nil
    }
})
```

Правила:

- `When`-ожидания проверяются **первыми** (с пропуском исчерпанных); `Run` —
  fallback для несовпавших вызовов. Частный кейс через `When` + общий через
  `Run` — штатная комбинация.
- Один `Run` на метод; повторный — ошибка настройки с обоими местами
  регистрации.
- `Run` не участвует в кардинальности: если нужен счётчик — считайте внутри
  функции и проверяйте в тесте, либо используйте `When`.

## История вызовов

```go
calls := m.GetCalls()       // []MockStoreGetParams — экспортируемые поля
n := m.GetCallsCount()
```

История фиксируется для всех вызовов (включая обслуженные `Run`-функцией).
Слайсы и мапы первого уровня клонируются в момент записи — мутация буфера
после вызова не меняет историю. Для горячих циклов запись истории отключается
опцией `history: false` генерации — вместе с методами `Calls`.

## Рецепты

**Ретраи через очередь ожиданий.** Исчерпанные ожидания пропускаются при
поиске, поэтому два одинаковых `When` с `Once()` дают последовательность:

```go
m.WhenGet(ctx, "k").Then("v1", nil).Once() // первая попытка
m.WhenGet(ctx, "k").Then("v2", nil).Once() // повтор после ретрая
```

**Out-параметр (`Read(buf []byte)`).** Буфер заполняет мок; кардинальности
у `Run` нет — считайте вызовы внутри:

```go
var readCalls int
m.RunRead(func(p []byte) (int, error) {
    readCalls++
    if readCalls == 1 {
        return copy(p, data), nil
    }
    return 0, io.EOF
})
n, err := io.Copy(dst, m) // мок — полноценный io.Reader
```

**Func-опции.** Сравнение функций невозможно — проверяйте их вызов через `Run`:

```go
m.RunPut(func(_ context.Context, key, val string, opts ...PutOption) error {
    require.Len(t, opts, 1)
    got := &PutOptions{}
    opts[0](got)
    require.True(t, got.Upsert)
    return nil
})
```

**История и переиспользуемые буферы.** Слайсы и мапы первого уровня (включая
именованные типы вроде `type IDs []string`) клонируются в момент записи —
код под тест может переиспользовать буфер, история не протухнет.

## Verify

Проверка ожиданий выполняется автоматически в `t.Cleanup`. Вручную —
`neomock.Verify(m)` (идемпотентно: двойной вызов не дублирует сообщения).
`Verify` проверяет только **нехватку** вызовов и `When` без `Then`;
unexpected-вызовы репортятся в момент вызова и повторно не упоминаются.

## Режимы ошибок

| Фаза | Поведение |
|---|---|
| Настройка (`Times(0)`, повторный `Run`, nil-fn) | всегда `Fatalf` |
| Вызов (unexpected, mismatch, нарушенный `Never`/`NeverXxx`) | soft: `Errorf` + zero values; если последний результат — `error`, возвращается ошибка, оборачивающая `neomock.ErrUnexpectedCall` (проверяется через `errors.Is`). strict: `Fatalf` |
| `Verify` | всегда `Errorf` (из `Cleanup` `Fatalf` некорректен) |

Soft-режим безопасен для моков, вызываемых из воркеров. Строгий включается
опцией конструктора `neomock.WithStrictCalls()` — только для одногортинных
тестов.

## Формат сообщений

Все сообщения — на английском, в стиле expected/got, с местом регистрации
ожидания:

```text
neomock: argument mismatch in Get call (ctx=..., key="k9"):
  arg[1] (key): expected "k1" (WhenGet at store_test.go:42), got "k9"

neomock: missing call(s): Get("k1") (WhenGet at store_test.go:42): expected at least 1 call(s), got 0
```

## Пакет рантайма

Сгенерированный код использует из пакета `neomock` ровно следующий API
(больше в рантайме нет ничего — zero-dependency):

| Сущность | Назначение |
|---|---|
| `Tester` | минимальный интерфейс тестера: `Helper/Cleanup/Errorf/Fatalf/Logf`; совместим с `*testing.T` |
| `Context` | сторожевой маркер wildcard-контекста для `When` |
| `Equal(expected, actual)` | сравнение аргументов (см. таблицу выше) |
| `Diff(expected, actual)` | построчный diff для mismatch-сообщений |
| `Verify(m)` | проверка кардинальности; паника на не-моке |
| `ErrUnexpectedCall` | sentinel для soft-режима; проверять через `errors.Is` |
| `WithStrictCalls()` | опция конструктора: строгий режим вызовов |
| `Caller(skip)` | `file:line` вызывающего кадра (origin ожиданий) |

## Дальше

- [Конфигурация](configuration.md) — файл конфига, все ключи и приоритеты.
- [CLI и генерация](cli.md) — команды, флаги, go:generate.
- [Примеры](examples.md) — шесть сценариев размещения моков и частные случаи.
- [Архитектура](architecture.md) — как устроен конвейер генерации и почему без `EXPECT`.
- [Разработка](development.md) — как перегенерировать golden-моки и запускать тесты.
