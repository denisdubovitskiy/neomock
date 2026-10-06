// Command neomock — генератор моков (DESIGN §5):
//
//	neomock <команда> [аргументы]
//	  init [--format yaml|toml] [--path PATH]
//	  generate [пакеты...] [--config PATH] [--clean]
//	  check [пакеты...] [--config PATH] [--output text|sarif]
//	  explain <пакет.Интерфейс>
//	  migrate --from .mockery.yaml --to .neomock.yml
//	  version
//
// Без команды: режим go:generate — с флагами (--interface ...) режим А,
// без аргументов режим Б (секция пакета текущего каталога, §4.3).
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"
)

const usage = `neomock — генератор моков для Go

Использование:
  neomock <команда> [аргументы]

Команды:
  init      [--format yaml|toml] [--path PATH]           создать файл конфигурации
  generate  [пакеты...] [--config PATH] [--clean]        сгенерировать моки
  check     [пакеты...] [--config PATH] [--output text|sarif]
                                                        проверить актуальность моков (exit 1 при расхождении)
  explain   <пакет.Интерфейс> [--config PATH]           показать значения и их источники
  migrate   --from .mockery.yaml --to .neomock.yml      перенести конфиг с mockery
  version                                                 показать версию

go:generate (внутри go generate):
  //go:generate go tool neomock --interface Store --out ./mocks/store_mock.go --pkgname mocks
  //go:generate go tool neomock    (режим Б: секция пакета из конфига)

Флаги go:generate: --interface (повторяется), --out, --pkgname, --structname,
--source, --inpackage, --strict-calls, --history, --build-tags, --boilerplate.

Переменные окружения: NEOMOCK_CONFIG, NEOMOCK_LOG_LEVEL (debug|info|warn|error).
`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	// режим go:generate: флаги без команды → режим А; пусто → режим Б (§4.3).
	if len(args) == 0 {
		return cmdGoGenerateB(nil)
	}
	if strings.HasPrefix(args[0], "-") {
		return cmdGoGenerateA(args)
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "generate":
		return cmdGenerate(rest)
	case "check":
		return cmdCheck(rest)
	case "explain":
		return cmdExplain(rest)
	case "init":
		return cmdInit(rest)
	case "migrate":
		return cmdMigrate(rest)
	case "version":
		fmt.Println(versionString())
		return 0
	case "help", "--help", "-h":
		fmt.Print(usage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "neomock: неизвестная команда %q\n\n%s", cmd, usage)
		return 2
	}
}

// versionString — версия из debug.ReadBuildInfo: работает и через go tool,
// и через tool-директиву без ldflags (план, открытое решение 4).
func versionString() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "neomock (unknown version)"
	}
	v := info.Main.Version
	if v == "" || v == "(devel)" {
		v = "devel"
	}
	return "neomock " + v
}

// newLogger — slog на stderr; уровень из --log-level / NEOMOCK_LOG_LEVEL (§4.6).
func newLogger(level string) *slog.Logger {
	if level == "" {
		level = os.Getenv("NEOMOCK_LOG_LEVEL")
	}
	var lv slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lv = slog.LevelDebug
	case "warn", "warning":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	return slog.New(
		slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lv}),
	)
}

// reportWarnings — предупреждения конвейера в журнал (§4.2).
func reportWarnings(log *slog.Logger, warnings []string) {
	for _, w := range warnings {
		log.Warn(w)
	}
}

// exitError — единая обработка ошибок команд.
func exitError(err error) int {
	if err != nil && !errors.Is(err, errSilent) {
		fmt.Fprintln(os.Stderr, err)
	}
	return 1
}

// errSilent — ошибка уже напечатана, молча выходим с кодом 1.
var errSilent = errors.New("silent")
