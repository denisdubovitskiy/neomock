package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// mockeryConfig — минимальное подмножество .mockery.yaml v3 для migrate (С10).
type mockeryConfig struct {
	Dir      string `yaml:"dir"`
	Packages map[string]struct {
		Config struct {
			All bool   `yaml:"all"`
			Dir string `yaml:"dir"`
		} `yaml:"config"`
		Interfaces map[string]struct {
			Config struct {
				StructName string `yaml:"structname"`
				Dir        string `yaml:"dir"`
			} `yaml:"config"`
		} `yaml:"interfaces"`
	} `yaml:"packages"`
}

// cmdMigrate — перенос конфига с mockery: пакеты/имена/пути + отчёт по
// ручному переписыванию тестов. Автопереписывания тестов нет (С10).
func cmdMigrate(args []string) int {
	from, to := "", ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--from", "-from":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "neomock: --from требует путь")
				return 2
			}
			i++
			from = args[i]
		case "--to", "-to":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "neomock: --to требует путь")
				return 2
			}
			i++
			to = args[i]
		default:
			fmt.Fprintf(
				os.Stderr,
				"neomock: migrate: неизвестный аргумент %q\n",
				args[i],
			)
			return 2
		}
	}
	if from == "" || to == "" {
		fmt.Fprintln(
			os.Stderr,
			"neomock: migrate требует --from <.mockery.yaml> и --to <.neomock.yml>",
		)
		return 2
	}
	data, err := os.ReadFile(from)
	if err != nil {
		fmt.Fprintf(os.Stderr, "neomock: %v\n", err)
		return 1
	}
	var mc mockeryConfig
	if err := yaml.Unmarshal(data, &mc); err != nil {
		fmt.Fprintf(os.Stderr, "neomock: не удалось разобрать %s: %v\n", from, err)
		return 1
	}
	if len(mc.Packages) == 0 {
		fmt.Fprintln(
			os.Stderr,
			"neomock: в конфиге нет секции packages — мигрировать нечего",
		)
		return 1
	}

	var b strings.Builder
	b.WriteString(
		"# Мигрировано из " + from + " — проверьте пути и имена перед первым прогоном.\n",
	)
	b.WriteString("version: 1\n\npackages:\n")
	for _, pkg := range sortedKeys(mc.Packages) {
		pc := mc.Packages[pkg]
		fmt.Fprintf(&b, "  %q:\n", pkg)
		if pc.Config.All && len(pc.Interfaces) == 0 {
			b.WriteString("    all: true\n")
			if d := firstNonEmpty(pc.Config.Dir, mc.Dir); d != "" {
				fmt.Fprintf(&b, "    config:\n      dir: %q\n", d)
			}
		} else if len(pc.Interfaces) > 0 {
			b.WriteString("    interfaces:\n")
			for _, name := range sortedKeys(pc.Interfaces) {
				ic := pc.Interfaces[name]
				d := firstNonEmpty(ic.Config.Dir, pc.Config.Dir, mc.Dir)
				if ic.Config.StructName == "" && d == "" {
					fmt.Fprintf(&b, "      %s: {}\n", name)
					continue
				}
				fmt.Fprintf(&b, "      %s:\n        config:\n", name)
				if d != "" {
					fmt.Fprintf(&b, "          dir: %q\n", d)
				}
				if ic.Config.StructName != "" {
					fmt.Fprintf(&b, "          structname: %q\n", ic.Config.StructName)
				}
			}
		} else {
			b.WriteString("    all: true # было неясно, какой селектор — проверьте\n")
		}
		b.WriteString("\n")
	}

	if err := os.WriteFile(to, []byte(b.String()), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "neomock: %v\n", err)
		return 1
	}
	fmt.Printf("created %s\n", to)
	fmt.Println(`
Отчёт по тестам (автопереписывание не делается, С10):
  - mock.On("Get", args...).Return(v, err)  →  m.WhenGet(ctx, "k").Then(v, err)
  - mock.Anything на позиции контекста      →  neomock.Context
  - EXPECT(*m).Get(...).Return(...)          →  m.WhenGet(...).Then(...)
  - RunAndReturn(fn) / .Run(fn)              →  m.RunGet(fn) — сигнатура в точности метода
  - .Times(n) / .Once() / .Maybe()           →  .Times(n) / .Once() / .Optional()
  - AssertExpectations                       →  не нужен: Verify в Cleanup конструктора
  - template: testify/matryer                →  удалено в ред. 2, When/Then единый стиль`)
	return 0
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
