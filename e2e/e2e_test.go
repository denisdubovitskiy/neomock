// Package e2e_test — сквозные сценарии C1–C11 (DESIGN §4.4) на реальном
// модуле-потребителе: собирается бинарь CLI, создаётся временный go-модуль
// с replace на репозиторий, прогоняются generate/check/migrate.
package e2e_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	binOnce sync.Once
	binPath string
	binErr  error
)

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	root, err := filepath.Abs(filepath.Join(wd, ".."))
	require.NoError(t, err)
	return root
}

// neomockBin — один собранный бинарь CLI на весь прогон.
func neomockBin(t *testing.T) string {
	t.Helper()
	binOnce.Do(func() {
		wd, err := os.Getwd()
		if err != nil {
			binErr = err
			return
		}
		root := filepath.Dir(wd)
		dir, err := os.MkdirTemp("", "neomock-bin")
		if err != nil {
			binErr = err
			return
		}
		binPath = filepath.Join(dir, "neomock")
		cmd := exec.Command("go", "build", "-o", binPath, "./cmd/neomock")
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			binErr = err
			binPath = string(out)
		}
	})
	require.NoErrorf(t, binErr, "сборка CLI: %s", binPath)
	return binPath
}

// mod — временный модуль-потребитель с replace на репозиторий.
type mod struct {
	t   *testing.T
	dir string
}

func newModule(t *testing.T) *mod {
	t.Helper()
	dir := t.TempDir()
	gomod := `module example.com/e2e

go 1.26.0

require github.com/denisdubovitskiy/neomock v0.0.0

replace github.com/denisdubovitskiy/neomock => ` + repoRoot(t) + "\n"
	writeFile(t, dir, "go.mod", gomod)
	writeFile(t, dir, "store/store.go", `package store

import "context"

type Store interface {
	Get(ctx context.Context, key string) (string, error)
	Put(ctx context.Context, key string, val []byte) error
}
`)
	return &mod{t: t, dir: dir}
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
}

func (m *mod) path(rel string) string { return filepath.Join(m.dir, rel) }

func (m *mod) read(rel string) string {
	m.t.Helper()
	data, err := os.ReadFile(m.path(rel))
	require.NoError(m.t, err)
	return string(data)
}

func (m *mod) exists(rel string) bool {
	m.t.Helper()
	_, err := os.Stat(m.path(rel))
	return err == nil
}

// run — neomock CLI в корне модуля.
func (m *mod) run(args ...string) (string, int) {
	m.t.Helper()
	return m.runIn(m.dir, args...)
}

// runIn — neomock CLI в указанном каталоге модуля.
func (m *mod) runIn(dir string, args ...string) (string, int) {
	m.t.Helper()
	cmd := exec.Command(neomockBin(m.t), args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		require.ErrorAs(m.t, err, &ee)
		code = ee.ExitCode()
	}
	m.t.Logf("neomock %v -> exit=%d\n%s", args, code, out)
	return string(out), code
}

// goCmd — go build/test/generate/tidy в модуле.
func (m *mod) goCmd(args ...string) (string, int) {
	m.t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = m.dir
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		require.ErrorAs(m.t, err, &ee)
		code = ee.ExitCode()
	}
	m.t.Logf("go %v -> exit=%d\n%s", args, code, out)
	return string(out), code
}

// TestC1_GoGenerateOnly — только //go:generate без файла конфига (С1).
func TestC1_GoGenerateOnly(t *testing.T) {
	t.Parallel()
	m := newModule(t)
	// arrange: tool-директива + режим А в //go:generate
	require.NoError(t, os.WriteFile(m.path("go.mod"),
		[]byte(m.read("go.mod")+"\ntool github.com/denisdubovitskiy/neomock/cmd/neomock\n"), 0o644))
	out, code := m.goCmd("mod", "tidy")
	require.Equalf(t, 0, code, "go mod tidy: %s", out)
	writeFile(t, m.dir, "store/gen.go", `package store

//go:generate go tool neomock --interface Store --out ./mocks/store_mock.go --pkgname mocks
`)

	// act
	out, code = m.goCmd("generate", "./store/")
	require.Equalf(t, 0, code, "go generate: %s", out)

	// assert
	require.FileExists(t, m.path("store/mocks/store_mock.go"))
	require.Contains(
		t,
		m.read("store/mocks/store_mock.go"),
		"//go:generate go tool neomock --interface Store",
	)
	out, code = m.goCmd("build", "./...")
	require.Equalf(t, 0, code, "сборка мока: %s", out)
}

// TestC2_MonorepoConfigAndCheck — all/include/exclude + check ловит устаревание (С2).
func TestC2_MonorepoConfigAndCheck(t *testing.T) {
	t.Parallel()
	m := newModule(t)
	writeFile(t, m.dir, "bus/bus.go", `package bus

type Producer interface{ Emit(e string) error }
type Consumer interface{ Consume() (string, error) }
`)
	writeFile(t, m.dir, ".neomock.yml", `version: 1
packages:
  "example.com/e2e/store":
    all: true
  "example.com/e2e/bus":
    include: "^Producer$"
`)

	// act/assert: генерация по селекторам
	out, code := m.run("generate")
	require.Equalf(t, 0, code, "generate: %s", out)
	require.True(t, m.exists("store/mocks/store_mock.go"))
	require.True(t, m.exists("bus/mocks/producer_mock.go"))
	require.False(
		t,
		m.exists("bus/mocks/consumer_mock.go"),
		"exclude/include: Consumer не должен генерироваться",
	)

	// check на свежих моках
	out, code = m.run("check")
	require.Equalf(t, 0, code, "check свежих: %s", out)

	// act: интерфейс изменился
	writeFile(t, m.dir, "store/store.go", `package store

type Store interface {
	Get(key string) (string, error)
}
`)
	out, code = m.run("check")
	require.Equal(t, 1, code)
	require.Contains(t, out, "stale")

	// regenerate → снова чисто
	_, code = m.run("generate")
	require.Equal(t, 0, code)
	_, code = m.run("check")
	require.Equal(t, 0, code)
}

// TestC3_PrivateInPackage — приватный интерфейс: без inpackage ошибка, с inpackage — mockfoo (С3).
func TestC3_PrivateInPackage(t *testing.T) {
	t.Parallel()
	m := newModule(t)
	writeFile(t, m.dir, "store/private.go", `package store

type refresher interface {
	Refresh() error
}
`)
	writeFile(t, m.dir, ".neomock.yml", `version: 1
packages:
  "example.com/e2e/store":
    interfaces:
      refresher: {}
`)

	// act: без inpackage — ошибка валидации
	out, code := m.run("generate")
	require.Equal(t, 1, code)
	require.Contains(t, out, "inpackage")

	// assert: с inpackage — мок в пакете, неэкспортированный конструктор
	writeFile(t, m.dir, ".neomock.yml", `version: 1
packages:
  "example.com/e2e/store":
    interfaces:
      refresher:
        config:
          inpackage: true
          dir: "{{.InterfaceDir}}"
          pkgname: store
          filename: "refresher_mock.go"
`)
	out, code = m.run("generate")
	require.Equalf(t, 0, code, "generate inpackage: %s", out)
	require.Contains(
		t,
		m.read("store/refresher_mock.go"),
		"func newMockrefresher(",
	)
	out, code = m.goCmd("build", "./...")
	require.Equalf(t, 0, code, "сборка: %s", out)
}

// TestC4_SourceTest — интерфейс только в *_test.go (С4).
func TestC4_SourceTest(t *testing.T) {
	t.Parallel()
	m := newModule(t)
	writeFile(t, m.dir, "store/clock_test.go", `package store

type FakeClock interface {
	Now() int64
}
`)
	writeFile(t, m.dir, ".neomock.yml", `version: 1
packages:
  "example.com/e2e/store":
    interfaces:
      FakeClock: {}
`)

	// act: source:go по умолчанию — понятная ошибка с подсказкой
	out, code := m.run("generate")
	require.Equal(t, 1, code)
	require.Contains(t, out, "не найден")
	require.Contains(t, out, "source")

	// assert: source:test + _test.go в имени
	writeFile(t, m.dir, ".neomock.yml", `version: 1
packages:
  "example.com/e2e/store":
    interfaces:
      FakeClock:
        config:
          source: test
          dir: "{{.InterfaceDir}}"
          pkgname: store
          filename: "fake_clock_mock_test.go"
`)
	out, code = m.run("generate")
	require.Equalf(t, 0, code, "generate source:test: %s", out)
	require.True(t, m.exists("store/fake_clock_mock_test.go"))
	out, code = m.goCmd("vet", "./...")
	require.Equalf(t, 0, code, "vet: %s", out)
}

// TestC5_ExternalPackage — мок io.Reader: dir внутрь модуля, Verify в тесте; inpackage — ошибка (С5).
func TestC5_ExternalPackage(t *testing.T) {
	t.Parallel()
	m := newModule(t)
	writeFile(t, m.dir, ".neomock.yml", `version: 1
packages:
  "io":
    interfaces:
      Reader:
        config:
          dir: "./internal/testmocks"
          pkgname: testmocks
          filename: "reader_mock.go"
`)

	// act
	out, code := m.run("generate")
	require.Equalf(t, 0, code, "generate io.Reader: %s", out)
	require.True(t, m.exists("internal/testmocks/reader_mock.go"))

	// assert: мок собирается и работает в тесте потребителя (Verify через Cleanup)
	writeFile(t, m.dir, "use_test.go", `package e2e_test

import (
	"errors"
	"io"
	"testing"

	"example.com/e2e/internal/testmocks"
)

func TestReaderMock(t *testing.T) {
	m := testmocks.NewMockReader(t)
	// сравнение []byte — точное (bytes.Equal), wildcard в рантайме только для ctx
	buf := make([]byte, 5)
	m.WhenRead(buf).Once().Then(5, nil)
	m.WhenRead(make([]byte, 5)).Once().Then(0, errors.New("eof"))

	n, err := m.Read(buf)
	if n != 5 || err != nil {
		t.Fatalf("got %d %v", n, err)
	}
	if _, err := m.Read(make([]byte, 5)); err == nil {
		t.Fatal("want eof")
	}
	var _ io.Reader = m
}
`)
	out, code = m.goCmd("test", "./...")
	require.Equalf(t, 0, code, "consumer test: %s", out)

	// assert: zero-dependency — в go.mod потребителя только neomock
	_, code = m.goCmd("mod", "tidy")
	require.Equal(t, 0, code)
	gomod := m.read("go.mod")
	require.NotContains(t, gomod, "testify")
	require.NotContains(t, gomod, "yaml")

	// act: inpackage для внешнего пакета — ошибка
	writeFile(t, m.dir, ".neomock.yml", `version: 1
packages:
  "io":
    interfaces:
      Reader:
        config:
          inpackage: true
`)
	out, code = m.run("generate")
	require.Equal(t, 1, code)
	require.Contains(t, out, "вне текущего модуля")
}

// TestC6_TwoMocks — два мока одного интерфейса через source-interface (С6).
func TestC6_TwoMocks(t *testing.T) {
	t.Parallel()
	m := newModule(t)
	writeFile(t, m.dir, ".neomock.yml", `version: 1
packages:
  "example.com/e2e/store":
    interfaces:
      Store: {}
      StoreV2:
        source-interface: Store
        config:
          structname: MockStoreV2
          filename: "store_v2_mock.go"
`)

	// act
	out, code := m.run("generate")
	require.Equalf(t, 0, code, "generate: %s", out)

	// assert
	require.True(t, m.exists("store/mocks/store_mock.go"))
	require.True(t, m.exists("store/mocks/store_v2_mock.go"))
	require.Contains(
		t,
		m.read("store/mocks/store_v2_mock.go"),
		"type MockStoreV2 struct",
	)
	out, code = m.goCmd("build", "./...")
	require.Equalf(t, 0, code, "сборка: %s", out)
}

// TestC10_Migrate — перенос .mockery.yaml + отчёт (С10).
func TestC10_Migrate(t *testing.T) {
	t.Parallel()
	m := newModule(t)
	writeFile(t, m.dir, ".mockery.yaml", `dir: ./mocks
packages:
  example.com/e2e/store:
    config:
      all: true
  io:
    interfaces:
      Reader:
        config:
          dir: ./internal/testmocks
          structname: MockReader
`)

	// act
	out, code := m.run(
		"migrate",
		"--from",
		".mockery.yaml",
		"--to",
		".neomock.yml",
	)
	require.Equalf(t, 0, code, "migrate: %s", out)

	// assert: валидный neomock-конфиг + отчёт по тестам
	require.Contains(t, out, "Отчёт по тестам")
	cfg := m.read(".neomock.yml")
	require.Contains(t, cfg, "all: true")
	require.Contains(t, cfg, "structname: \"MockReader\"")
	out, code = m.run("generate")
	require.Equalf(t, 0, code, "generate после migrate: %s", out)
	require.True(t, m.exists("internal/testmocks/reader_mock.go"))
}

// TestC11_OrphanedClean — orphaned-скан, --clean, отказ при частичном прогоне (С11).
func TestC11_OrphanedClean(t *testing.T) {
	t.Parallel()
	m := newModule(t)
	writeFile(t, m.dir, ".neomock.yml", `version: 1
packages:
  "example.com/e2e/store":
    all: true
`)
	_, code := m.run("generate")
	require.Equal(t, 0, code)

	// act: лишний мок с маркером в целевом каталоге (не из текущего конфига) —
	// например, остался от удалённой go:generate-строки режима А
	_, code = m.runIn(m.path("store"), "--interface", "Store",
		"--out", "./mocks/store_strict_mock.go", "--pkgname", "mocks", "--structname", "MockStoreStrict")
	require.Equal(t, 0, code)
	require.True(t, m.exists("store/mocks/store_strict_mock.go"))

	out, code := m.run("check")
	require.Equal(t, 0, code)
	require.Contains(t, out, "orphaned")
	require.Contains(t, out, "store_strict_mock.go")

	// act: частичный прогон с --clean — отказ
	out, code = m.run("generate", "--clean", "./store")
	require.Equal(t, 1, code)
	require.Contains(t, out, "--clean")

	// assert: полный --clean удаляет осиротевший файл
	out, code = m.run("generate", "--clean")
	require.Equalf(t, 0, code, "clean: %s", out)
	require.Contains(t, out, "deleted")
	require.False(t, m.exists("store/mocks/store_strict_mock.go"))
	require.True(t, m.exists("store/mocks/store_mock.go"))
}

// TestManualFileUntouched — файл без маркера не перезаписывается и не удаляется (§3.2).
func TestManualFileUntouched(t *testing.T) {
	t.Parallel()
	m := newModule(t)
	writeFile(
		t,
		m.dir,
		"store/mocks/store_mock.go",
		"package mocks\n\n// ручной файл без маркера\n",
	)
	writeFile(t, m.dir, ".neomock.yml", `version: 1
defaults:
  force-write: false
packages:
  "example.com/e2e/store":
    all: true
`)

	// act
	out, code := m.run("generate")
	require.Equalf(t, 0, code, "generate: %s", out)

	// assert: force-write:false сохраняет ручную правку
	require.Equal(
		t,
		"package mocks\n\n// ручной файл без маркера\n",
		m.read("store/mocks/store_mock.go"),
	)
	require.Contains(t, out, "skipped")
}

// TestC5b_ExternalDeps — http.ResponseWriter (GOROOT) и внешняя зависимость
// через replace: генерация, сборка, Verify (С5/С5а).
func TestC5b_ExternalDeps(t *testing.T) {
	t.Parallel()
	m := newModule(t)
	// arrange: локальный «внешний» модуль-зависимость
	dep := t.TempDir()
	writeFile(t, dep, "go.mod", "module example.com/dep\n\ngo 1.26.0\n")
	writeFile(t, dep, "api/api.go", `package api

type Pinger interface {
	Ping() (string, error)
}
`)
	writeFile(t, m.dir, "use.go", `package e2e

import _ "example.com/dep/api"
`)
	require.NoError(t, os.WriteFile(m.path("go.mod"),
		[]byte(m.read("go.mod")+`
require example.com/dep v0.0.0

replace example.com/dep => `+dep+"\n"), 0o644))
	writeFile(t, m.dir, ".neomock.yml", `version: 1
packages:
  "net/http":
    interfaces:
      ResponseWriter:
        config:
          dir: "./internal/testmocks"
          pkgname: testmocks
          filename: "response_writer_mock.go"
  "example.com/dep/api":
    interfaces:
      Pinger:
        config:
          dir: "./internal/testmocks"
          pkgname: testmocks
          filename: "pinger_mock.go"
`)

	// act
	out, code := m.goCmd("mod", "tidy")
	require.Equalf(t, 0, code, "tidy: %s", out)
	out, code = m.run("generate")
	require.Equalf(t, 0, code, "generate: %s", out)

	// assert
	require.True(t, m.exists("internal/testmocks/response_writer_mock.go"))
	require.True(t, m.exists("internal/testmocks/pinger_mock.go"))
	out, code = m.goCmd("mod", "tidy")
	require.Equalf(t, 0, code, "tidy после generate: %s", out)
	writeFile(t, m.dir, "use_test.go", `package e2e_test

import (
	"testing"

	"example.com/e2e/internal/testmocks"
)

func TestPinger(t *testing.T) {
	m := testmocks.NewMockPinger(t)
	m.WhenPing().Then("pong", nil)
	s, err := m.Ping()
	if s != "pong" || err != nil {
		t.Fatalf("got %q %v", s, err)
	}
}
`)
	out, code = m.goCmd("test", "./...")
	require.Equalf(t, 0, code, "consumer test: %s", out)
}

// TestSARIF — check --output sarif: валидный SARIF 2.1.0 с правилом stale-mock (С11).
func TestSARIF(t *testing.T) {
	t.Parallel()
	m := newModule(t)
	writeFile(t, m.dir, ".neomock.yml", `version: 1
packages:
  "example.com/e2e/store":
    all: true
`)
	_, code := m.run("generate")
	require.Equal(t, 0, code)
	writeFile(
		t,
		m.dir,
		"store/store.go",
		"package store\n\ntype Store interface{ Get() error }\n",
	)

	out, code := m.run("check", "--output", "sarif")
	require.Equal(t, 1, code)

	var doc struct {
		Version string `json:"version"`
		Runs    []struct {
			Tool struct {
				Driver struct {
					Rules []struct {
						ID string `json:"id"`
					} `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
			Results []struct {
				RuleID string `json:"ruleId"`
			} `json:"results"`
		} `json:"runs"`
	}
	require.NoErrorf(
		t,
		json.Unmarshal([]byte(out), &doc),
		"SARIF невалиден: %s",
		out,
	)
	require.Equal(t, "2.1.0", doc.Version)
	require.Len(t, doc.Runs, 1)
	require.Equal(t, "neomock/stale-mock", doc.Runs[0].Tool.Driver.Rules[0].ID)
	require.Len(t, doc.Runs[0].Results, 1)
	require.Equal(t, "neomock/stale-mock", doc.Runs[0].Results[0].RuleID)
}

// TestIdempotentMtime — повторный прогон не трогает mtime (skip-write-if-equal).
func TestIdempotentMtime(t *testing.T) {
	t.Parallel()
	m := newModule(t)
	writeFile(t, m.dir, ".neomock.yml", `version: 1
packages:
  "example.com/e2e/store":
    all: true
`)
	_, code := m.run("generate")
	require.Equal(t, 0, code)
	info1, err := os.Stat(m.path("store/mocks/store_mock.go"))
	require.NoError(t, err)

	_, code = m.run("generate")
	require.Equal(t, 0, code)
	info2, err := os.Stat(m.path("store/mocks/store_mock.go"))
	require.NoError(t, err)
	require.True(
		t,
		info1.ModTime().Equal(info2.ModTime()),
		"mtime изменился при равных байтах",
	)
}

// TestGoWork — генерация в модуле внутри go.work (§3.3).
func TestGoWork(t *testing.T) {
	t.Parallel()
	m := newModule(t)
	writeFile(t, m.dir, "go.work", "go 1.26.0\n\nuse .\n")
	writeFile(t, m.dir, ".neomock.yml", `version: 1
packages:
  "example.com/e2e/store":
    all: true
`)

	out, code := m.run("generate")
	require.Equalf(t, 0, code, "generate в go.work: %s", out)
	require.True(t, m.exists("store/mocks/store_mock.go"))
	out, code = m.goCmd("build", "./...")
	require.Equalf(t, 0, code, "сборка: %s", out)
}

// TestVendor — генерация мока внешней зависимости в vendored-режиме (§3.3).
func TestVendor(t *testing.T) {
	t.Parallel()
	m := newModule(t)
	dep := t.TempDir()
	writeFile(t, dep, "go.mod", "module example.com/dep\n\ngo 1.26.0\n")
	writeFile(t, dep, "api/api.go", `package api

type Pinger interface {
	Ping() (string, error)
}
`)
	require.NoError(t, os.WriteFile(m.path("go.mod"),
		[]byte(m.read("go.mod")+`
require example.com/dep v0.0.0

replace example.com/dep => `+dep+"\n"), 0o644))
	writeFile(t, m.dir, "use.go", `package e2e

import _ "example.com/dep/api"
`)
	writeFile(t, m.dir, ".neomock.yml", `version: 1
packages:
  "example.com/dep/api":
    interfaces:
      Pinger:
        config:
          dir: "./internal/testmocks"
          pkgname: testmocks
          filename: "pinger_mock.go"
`)

	// act
	out, code := m.goCmd("mod", "tidy")
	require.Equalf(t, 0, code, "tidy: %s", out)
	out, code = m.goCmd("mod", "vendor")
	require.Equalf(t, 0, code, "vendor: %s", out)
	out, code = m.run("generate")
	require.Equalf(t, 0, code, "generate в vendor: %s", out)

	// assert
	require.True(t, m.exists("vendor/example.com/dep/api/api.go"))
	require.True(t, m.exists("internal/testmocks/pinger_mock.go"))
	// мок импортирует neomock — пересобираем vendor с ним (реальный workflow:
	// в сервисе neomock уже заведен, здесь — добавляем после первой генерации)
	out, code = m.goCmd("mod", "tidy")
	require.Equalf(t, 0, code, "tidy после generate: %s", out)
	out, code = m.goCmd("mod", "vendor")
	require.Equalf(t, 0, code, "vendor после generate: %s", out)
	out, code = m.goCmd("build", "./...")
	require.Equalf(t, 0, code, "сборка: %s", out)
}

// TestGenerateLineOff — generate-line: false: в файле нет //go:generate-шапки,
// остальной код на месте.
func TestGenerateLineOff(t *testing.T) {
	t.Parallel()
	m := newModule(t)
	writeFile(t, m.dir, ".neomock.yml", `version: 1
defaults:
  generate-line: false
packages:
  "example.com/e2e/store":
    all: true
`)

	// act
	out, code := m.run("generate")
	require.Equalf(t, 0, code, "generate: %s", out)

	// assert
	content := m.read("store/mocks/store_mock.go")
	require.NotContains(t, content, "//go:generate")
	require.Contains(t, content, "// Code generated by neomock DO NOT EDIT.")
	require.Contains(t, content, "type MockStore struct")
	out, code = m.goCmd("build", "./...")
	require.Equalf(t, 0, code, "сборка мока: %s", out)
}
