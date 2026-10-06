---
title: Примеры
---

# Примеры

Готовые рецепты на все типовые случаи. Размещение мока выбирается по вопросу
«**откуда интерфейс** и **куда кладём мок**» — шесть основных сценариев ниже,
затем частные случаи. Настройки и их приоритет — в [конфигурации](configuration.md),
команды — в [CLI](cli.md), DSL в тестах — в [использовании](usage.md).

## Шесть основных сценариев

| Откуда интерфейс | Куда мок | Ключевые настройки |
|---|---|---|
| стандартная библиотека (`io`) | пакет внутри проекта | `dir` внутрь модуля + `pkgname` |
| сторонняя зависимость (`github.com/...`) | пакет внутри проекта | `dir` внутрь модуля + `pkgname` |
| другой пакет своего проекта | `mocks` рядом с ним | дефолты — ничего настраивать не нужно |
| текущий пакет своего проекта | тот же пакет | `inpackage: true` |
| любой | **в текущую директорию** | `dir: "{{.InterfaceDir}}"` (+ `inpackage` или свой `pkgname`) |
| любой | **в отдельный пакет** | дефолт `{{.InterfaceDir}}/mocks` или свой `dir` |

!!! note "Правило размещения"
    Каталог мока всегда обязан лежать **внутри текущего модуля**. Для
    стандартной библиотеки и сторонних модулей это единственная причина
    задавать `dir` явно — физически записать файл в чужой модуль нельзя.

### 1. Мок из стандартной библиотеки

Интерфейсы `io`, `net/http`, `database/sql` — вне нашего модуля, поэтому
указываем каталог внутри проекта и имя пакета мока:

```yaml
# .neomock.yml
version: 1
packages:
  "io":
    interfaces:
      Reader:
        config:
          dir: "./internal/testmocks"
          pkgname: testmocks
      Writer:
        config:
          dir: "./internal/testmocks"
          pkgname: testmocks
```

```console
$ go tool neomock generate
written  /home/dev/proj/internal/testmocks/reader_mock.go
written  /home/dev/proj/internal/testmocks/writer_mock.go
```

Использование — мок полноценный `io.Reader`, работает с `io.Copy`:

```go
import testmocks "example.com/proj/internal/testmocks"

func TestUpload(t *testing.T) {
    m := testmocks.NewMockReader(t)
    // общий случай: мок отдаёт data, затем EOF
    var once bool
    m.RunRead(func(p []byte) (int, error) {
        if once {
            return 0, io.EOF
        }
        once = true
        return copy(p, data), nil
    })
    n, err := io.Copy(dst, m) // мок — полноценный io.Reader
}
```

Для `net/http.ResponseWriter` — тот же приём; мок кладётся в свой пакет внутри
модуля (`dir: "./internal/testmocks"`, `pkgname: testmocks`).

### 2. Мок из сторонней зависимости

Ситуация идентична стандартной библиотеке: чужой модуль → `dir` внутрь своего
проекта. Отличие только в import path секции:

```yaml
version: 1
packages:
  "github.com/example/payments-sdk-go":
    interfaces:
      Charger:
        config:
          dir: "./internal/testmocks"
          pkgname: testmocks
          structname: "MockCharger"
```

```go
m := testmocks.NewMockCharger(t)
m.WhenCharge(ctx, "order-1", 499).Then(&payments.Receipt{ID: "rcp_1"}, nil)
```

!!! tip
    Если моков из внешних пакетов много, заведите один общий пакет
    `internal/testmocks` (или несколько по доменам) — так у моков стабильный
    import path и нет коллизий имён структур.

### 3. Мок из другого пакета своего проекта

Самый частый случай — дефолты уже делают нужное: `dir = {{.InterfaceDir}}/mocks`,
`pkgname = mocks`, `filename = {{.InterfaceSnake}}_mock.go`:

```yaml
version: 1
packages:
  "example.com/proj/internal/store":
    all: true
```

```console
$ go tool neomock generate
written  /home/dev/proj/internal/store/mocks/store_mock.go
```

```go
import "example.com/proj/internal/store/mocks"

func TestService(t *testing.T) {
    m := mocks.NewMockStore(t)
    m.WhenGet(ctx, "k1").Then("v1", nil)
    svc := NewService(m) // мок удовлетворяет store.Store
}
```

Несколько пакетов — несколько секций; фильтр запуска — позиционные аргументы:

```console
$ go tool neomock generate example.com/proj/internal/store
```

### 4. Мок из текущего пакета (inpackage)

Мок генерируется **в тот же пакет**, что и интерфейс — без отдельного пакета и
без импорта. Типичный кейс — приватный интерфейс, который нельзя вынести:

```yaml
version: 1
packages:
  "example.com/proj/internal/cache":
    interfaces:
      refresher:
        config:
          inpackage: true
          pkgname: ""                 # дефолт mocks конфликтует с inpackage
          dir: "{{.InterfaceDir}}"
          filename: "refresher_mock.go"
```

Результат — `internal/cache/refresher_mock.go` в `package cache` со структурой
`mockrefresher` и конструктором `newMockrefresher` (префикс `mock` и
неэкспортируемое имя — для приватного интерфейса). Тесты пакета используют его
напрямую:

```go
// internal/cache/refresher_test.go — package cache
func TestRefresher(t *testing.T) {
    m := newMockrefresher(t)
    m.WhenRefresh(ctx).Then(nil)
}
```

!!! warning "inpackage и прод-сборка"
    Файл мока без суффикса `_test.go` компилируется в прод-сборку пакета.
    Если мок нужен только в тестах этого же пакета, используйте
    `filename: "refresher_mock_test.go"` — мок станет тестовым файлом пакета.

### 5. Генерация моков в текущую директорию

«Текущая директория» = каталог пакета источника. Два варианта:

=== "В тот же пакет (inpackage)"

    ```yaml
    packages:
      "example.com/proj/internal/store":
        all: true
        config:
          inpackage: true
          pkgname: ""
          dir: "{{.InterfaceDir}}"
          filename: "{{.InterfaceSnake}}_mock_test.go"
    ```

=== "В подкаталог mocks (дефолт)"

    ```yaml
    packages:
      "example.com/proj/internal/store":
        all: true
        config:
          dir: "{{.InterfaceDir}}/mocks"   # он же и есть default
          pkgname: mocks
    ```

Через `go:generate` (режим А) — файл задаётся прямо в строке, конфиг не нужен:

```go
//go:generate go tool neomock --interface Store --out ./store_mock.go --inpackage --pkgname store
```

### 6. Генерация моков в отдельный пакет

Вариант «все моки проекта в одном пакете» — единый `dir` в `defaults`:

```yaml
version: 1
defaults:
  dir: "./internal/mocks"
  pkgname: mocks
packages:
  "example.com/proj/internal/store":
    all: true
  "example.com/proj/internal/cache":
    all: true
```

Все моки лягут в `internal/mocks` (`store_mock.go`, `cache_mock.go`, ...),
импорт — один на весь проект:

```go
import mocks "example.com/proj/internal/mocks"
```

!!! tip "Коллизии имён"
    В общем пакете имена файлов (`{{.InterfaceSnake}}_mock.go`) и структур
    (`Mock{{.InterfaceName}}`) должны быть уникальны. При коллизиях —
    уточните `filename`/`structname` на уровне пакета или интерфейса,
    либо разнесите моки по пакетам доменов.

## Частные случаи

### Приватный интерфейс

Дефолтный `structname` подстраивается: `Mock` → `mock`, конструктор —
`newMockrefresher`. См. сценарий 4.

### Дженерик-интерфейс

```go
type Store[T any] interface {
    Get(ctx context.Context, id string) (T, error)
}
```

Мок генерируется параметрическим; в тестах инстанцируется явно:

```go
m := mocks.NewMockGeneric[int](t)
m.WhenGet(ctx, "k").Then(42, nil)
```

### Variadic-метод

```go
type Store interface {
    Put(ctx context.Context, key, val string, opts ...PutOption) error
}
```

Variadic пробрасывается как есть (`opts ...PutOption`). Точное сравнение
func-опций невозможно — для проверки опций используйте `Run`
(см. [рецепты DSL](usage.md)).

Живой пример с тестами — `example/yaml/variadic` в репозитории:
фиксированные аргументы + variadic, только variadic, variadic из
именованного типа, `Run` и история вызовов.

### Встраивания и ромбовидные интерфейсы

```go
type ReadWriter interface {
    io.Reader
    io.Writer
}
```

Методы разворачиваются в плоский список, дубликаты при ромбе (`A embeds B, C;
B, C embed D`) дедуплицируются. Мок `MockReadWriter` реализует и `io.Reader`,
и `io.Writer`.

### Интерфейс из _test.go (source: test)

```yaml
packages:
  "example.com/proj/internal/api":
    interfaces:
      TestOnly:
        config:
          source: test
          dir: "{{.InterfaceDir}}"
          filename: "test_only_mock_test.go"
```

`source: test` грузит только `_test.go` пакета; `all` — и исходники, и тесты.
Файл мока обязан оканчиваться на `_test.go`, иначе генератор предупредит о
попадании в прод-сборку.

### source-interface (алиас источника)

```yaml
packages:
  "example.com/proj/internal/db":
    interfaces:
      UserStore:
        source-interface: Store   # мок Store[User] под именем UserStore
```

### include / exclude (regexp)

```yaml
packages:
  "example.com/proj/internal/repo":
    include: "Repo$"        # все XxxRepo
    exclude: "Legacy"       # ...кроме LegacyXxx
```

### Рекурсивный обход /...

```yaml
packages:
  "example.com/proj/internal/...":
    all: true
    config:
      recursive: true
      exclude-packages:
        - "example.com/proj/internal/generated/.*"
```

Только свой модуль; без `recursive: true` — ошибка.

### Теги сборки

```yaml
defaults:
  build-tags: "integration,cgo"
```

Теги применяются при загрузке пакета источника (`go/packages -tags`).

### Кастомные имена

```yaml
defaults:
  dir: "{{.InterfaceDirRelative}}/testmocks"
  filename: "mock_{{.InterfaceSnake}}.go"
  structname: "{{.InterfaceName}}Mock"   # StoreMock вместо MockStore
```

### Лицензионная шапка (boilerplate)

```yaml
defaults:
  template-data:
    boilerplate-file: "./lic_header.txt"
```

Содержимое файла вставляется перед маркером `// Code generated by neomock`.

### Отключить историю вызовов

```yaml
packages:
  "example.com/proj/internal/hotpath":
    all: true
    config:
      history: false
```

Не генерируются `XxxCalls`/`XxxCallsCount` и запись истории — меньше аллокаций
в горячих тестах.

### Строгий режим вызовов

```yaml
defaults:
  strict-calls: true
```

Unexpected-вызовы падают через `Fatalf` вместо мягкого `Errorf`. Только для
одногортинных тестов; локально — опция конструктора `neomock.WithStrictCalls()`.

### Ручные правки не затираются

```yaml
defaults:
  force-write: false
```

Сгенерированный файл с ручной правкой остаётся нетронутым (статус `skipped`),
расхождение видно в `check`.

### Проверка сборки мока

```yaml
defaults:
  verify-build: true
```

После записи — `go build` пакета мока; битый генерат не «протечёт» в коммит.

### CI: проверка актуальности

```yaml
# .github/workflows/ci.yml
- run: go tool neomock check --output sarif > neomock.sarif
```

exit 1 при устаревших/отсутствующих моках; SARIF 2.1.0 с правилом
`neomock/stale-mock` для встраивания в отчёты.

### Удаление осиротевших моков

```console
$ go tool neomock generate --clean
deleted  /home/dev/proj/internal/store/mocks/old_iface_mock.go
```

Только полный прогон с файлом конфигурации; удаляются лишь файлы с маркером
`Code generated by neomock`, не соответствующие ни одному интерфейсу из конфига.

### go:generate без конфига (режим А)

```go
//go:generate go tool neomock --interface Store --out ./mocks/store_mock.go --pkgname mocks
```

```console
$ go generate ./internal/store/
```

### go:generate с конфигом (режим Б)

```go
//go:generate go tool neomock
```

Все детали — из секции пакета текущего каталога; конфиг ищется от cwd вверх.

### Миграция с mockery

```console
$ go tool neomock migrate --from .mockery.yaml --to .neomock.yml
```

Конфиг переносится автоматически, тесты — по таблице соответствий из вывода
команды (см. [CLI](cli.md#migrate)).

## Дальше

- [Конфигурация](configuration.md) — полная схема и приоритеты.
- [CLI и генерация](cli.md) — команды и флаги.
- [Использование](usage.md) — DSL мока: When/Then, кардинальность, Run, история.
