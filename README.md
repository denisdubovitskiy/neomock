# neomock

Генератор моков для Go с собственным zero-dependency рантаймом. Альтернатива
mockery/minimock/gomock: типизированный DSL `When/Then/Run` без строковых имён
методов и `any`-аргументов, точечный wildcard для контекста вместо
`mock.Anything`, совместимая с `require.Equal` семантика сравнения без импорта
testify.

```go
m := mocks.NewMockStore(t)

m.WhenGet(ctx, "k1").Then("v1", nil)          // точное ожидание
m.WhenGet(neomock.Context, "any").Then("v", nil) // контекст не матчится
m.WhenGet(ctx, "k").Then("v", nil).Once()     // кардинальность
m.NeverDelete()                               // метод запрещён целиком

m.RunGet(func(ctx context.Context, key string) (string, error) {
    // сложная логика — обычный Go-код вместо матчеров
    switch key {
    case "":
        return "", store.ErrNotFound
    default:
        return "v:" + key, nil
    }
})
```

## Установка

Через `tool`-директиву Go 1.24+ (`go.mod` потребителя, модуль neomock требует Go 1.26+):

```
tool github.com/denisdubovitskiy/neomock/cmd/neomock
```

Далее `go tool neomock <команда>`. Сгенерированные моки и рантайм
zero-dependency: testify потребителю не нужен.

## Конфигурация и CLI

Файл `.neomock.yml` / `.neomock.toml` в корне модуля (`go tool neomock init`
создаст скелет) или `//go:generate`-строки в коде:

```yaml
version: 1
packages:
  "example.com/proj/internal/store":
    all: true
  "io":
    interfaces:
      Reader:
        config:
          dir: "./internal/testmocks"
          pkgname: testmocks
```

Команды:

- `generate [пакеты...] [--clean]` — генерация; запись только при расхождении
  байтов (mtime не меняется); `--clean` удаляет orphaned-моки.
- `check [пакеты...] [--output text|sarif]` — проверка в CI: unified diff
  устаревших моков, exit 1; orphaned-репорт; SARIF 2.1.0.
- `explain <пакет.Интерфейс>` — значение каждой опции и её источник.
- `init [--format yaml|toml] [--path PATH]` — скелет конфига.
- `migrate --from .mockery.yaml --to .neomock.yml` — перенос конфига mockery
  + отчёт по переписыванию тестов.
- `version`.

`go:generate` — два режима: `//go:generate go tool neomock` (секция пакета из
файла) и с флагами (`--interface`, `--out`, `--pkgname`, ...) без конфига.

Подробнее:

- **Документация** — [docs/](docs/index.md): [использование](docs/usage.md),
  [конфигурация](docs/configuration.md), [архитектура](docs/architecture.md),
  [разработка](docs/development.md).

## Разработка

```sh
go build ./... && go vet ./... && go test -race ./...   # полная проверка
NEOMOCK_UPDATE_GOLDEN=1 go test ./internal/emitter      # регенерация golden-моков
make lint                                                # golangci-lint (в Docker)
make act                                                 # CI локально через act
make docs-serve                                          # документация (poetry)
```
