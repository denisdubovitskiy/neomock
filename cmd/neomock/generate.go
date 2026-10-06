package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/denisdubovitskiy/neomock/internal/gen"
)

// commonArgs — разобранные аргументы generate/check.
type commonArgs struct {
	patterns   []string
	configPath string
	logLevel   string
	clean      bool
	sarif      bool
}

// parseCommon разбирает [пакеты...] [--config PATH] [--log-level L] [--clean]
// [--output text|sarif]. Собственный разбор: флаги могут стоять и после
// позиционных аргументов (go generate передаёт их в любом порядке).
func parseCommon(args []string) (*commonArgs, error) {
	c := &commonArgs{}
	value := func(i *int, name string) (string, error) {
		if *i+1 >= len(args) {
			return "", fmt.Errorf("neomock: флаг %s требует значение", name)
		}
		*i++
		return args[*i], nil
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		name, val, hasVal := splitFlag(a)
		switch name {
		case "--config":
			v, err := flagValue(&i, name, val, hasVal, value)
			if err != nil {
				return nil, err
			}
			c.configPath = v
		case "--log-level":
			v, err := flagValue(&i, name, val, hasVal, value)
			if err != nil {
				return nil, err
			}
			c.logLevel = v
		case "--output":
			v, err := flagValue(&i, name, val, hasVal, value)
			if err != nil {
				return nil, err
			}
			switch v {
			case "text":
			case "sarif":
				c.sarif = true
			default:
				return nil, fmt.Errorf(
					"neomock: --output: допустимы text|sarif, получено %q",
					v,
				)
			}
		case "--clean":
			c.clean = true
		default:
			if hasVal {
				return nil, fmt.Errorf("neomock: неизвестный флаг %q", name)
			}
			c.patterns = append(c.patterns, a)
		}
	}
	return c, nil
}

func splitFlag(a string) (name, val string, hasVal bool) {
	if len(a) < 2 || a[0] != '-' {
		return a, "", false
	}
	name = a
	if eq := indexByte(a, '='); eq >= 0 {
		name, val, hasVal = a[:eq], a[eq+1:], true
	}
	return name, val, hasVal
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func flagValue(
	i *int,
	name,
	val string,
	hasVal bool,
	next func(*int, string) (string, error),
) (string, error) {
	if hasVal {
		return val, nil
	}
	return next(i, name)
}

func cmdGenerate(args []string) int {
	c, err := parseCommon(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	log := newLogger(c.logLevel)

	if c.clean && len(c.patterns) > 0 {
		fmt.Fprintln(
			os.Stderr,
			"neomock: --clean работает только при полном прогоне без фильтра пакетов: "+
				"при частичном запуске общие каталоги содержат чужие моки, их удаление было бы потерей данных (§5/С11)",
		)
		return 1
	}

	cwd, err := os.Getwd()
	if err != nil {
		return exitError(err)
	}
	res, err := gen.Run(gen.ModeGenerate, cwd, gen.Options{
		ConfigPath: c.configPath,
		Patterns:   c.patterns,
		Clean:      c.clean,
	})
	if res != nil {
		reportWarnings(log, res.Warnings)
		for _, f := range res.Files {
			fmt.Printf("%-10s %s\n", f.Status, f.Path)
		}
		for _, d := range res.Deleted {
			fmt.Printf("%-10s %s\n", "deleted", d)
		}
		if res.Notice != "" {
			log.Info(res.Notice)
		}
	}
	if err != nil {
		return exitError(err)
	}
	return 0
}

func cmdCheck(args []string) int {
	c, err := parseCommon(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	log := newLogger(c.logLevel)

	cwd, err := os.Getwd()
	if err != nil {
		return exitError(err)
	}
	res, err := gen.Run(gen.ModeCheck, cwd, gen.Options{
		ConfigPath: c.configPath,
		Patterns:   c.patterns,
	})
	if err != nil && res == nil {
		return exitError(err)
	}
	if res != nil {
		reportWarnings(log, res.Warnings)
		if c.sarif {
			fmt.Print(renderSARIF(res))
		} else {
			for _, f := range res.Files {
				if f.Missing {
					fmt.Printf("missing   %s\n", f.Path)
					fmt.Print(f.Diff)
					continue
				}
				if f.Diff != "" {
					fmt.Printf("stale     %s\n", f.Path)
					fmt.Print(f.Diff)
				}
			}
			if res.Stale == 0 {
				fmt.Println("all mocks are up to date")
			}
			if len(res.Orphaned) > 0 {
				fmt.Println("orphaned mock files:")
				for _, o := range res.Orphaned {
					fmt.Printf("  %s\n", o)
				}
			}
			if res.Notice != "" {
				log.Info(res.Notice)
			}
		}
	}
	if err != nil {
		return exitError(err)
	}
	if res != nil && res.Stale > 0 {
		return 1
	}
	return 0
}

// renderSARIF — минимальный SARIF 2.1.0: одно правило neomock/stale-mock,
// по одному result на устаревший файл, region.startLine: 1 (§4.4/С11).
func renderSARIF(res *gen.Result) string {
	type artifactLocation struct {
		URI string `json:"uri"`
	}
	type region struct {
		StartLine int `json:"startLine"`
	}
	type physicalLocation struct {
		ArtifactLocation artifactLocation `json:"artifactLocation"`
		Region           region           `json:"region"`
	}
	type location struct {
		PhysicalLocation physicalLocation `json:"physicalLocation"`
	}
	type textMessage struct {
		Text string `json:"text"`
	}
	type sarifResult struct {
		RuleID    string      `json:"ruleId"`
		Message   textMessage `json:"message"`
		Locations []location  `json:"locations"`
	}
	type rule struct {
		ID                   string      `json:"id"`
		ShortDescription     textMessage `json:"shortDescription"`
		DefaultConfiguration struct {
			Level string `json:"level"`
		} `json:"defaultConfiguration"`
	}
	type driver struct {
		Name  string `json:"name"`
		Rules []rule `json:"rules"`
	}
	type tool struct {
		Driver driver `json:"driver"`
	}
	type run struct {
		Tool    tool          `json:"tool"`
		Results []sarifResult `json:"results"`
	}
	type report struct {
		Schema  string `json:"$schema"`
		Version string `json:"version"`
		Runs    []run  `json:"runs"`
	}

	rep := report{
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Version: "2.1.0",
		Runs: []run{{
			Tool: tool{Driver: driver{
				Name: "neomock",
				Rules: []rule{{
					ID:               "neomock/stale-mock",
					ShortDescription: textMessage{Text: "Generated mock is out of date"},
				}},
			}},
			Results: []sarifResult{},
		}},
	}
	rep.Runs[0].Tool.Driver.Rules[0].DefaultConfiguration.Level = "error"
	add := func(path, msg string) {
		rep.Runs[0].Results = append(rep.Runs[0].Results, sarifResult{
			RuleID:  "neomock/stale-mock",
			Message: textMessage{Text: msg},
			Locations: []location{{PhysicalLocation: physicalLocation{
				ArtifactLocation: artifactLocation{URI: path},
				Region:           region{StartLine: 1},
			}}},
		})
	}
	for _, f := range res.Files {
		if f.Missing {
			add(f.Path, "mock file is missing: run `go tool neomock generate`")
		} else if f.Diff != "" {
			add(f.Path, "mock is out of date: run `go tool neomock generate`")
		}
	}
	for _, o := range res.Orphaned {
		add(o, "orphaned mock file: no configured interface matches it")
	}
	out, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return "{}\n"
	}
	return string(out) + "\n"
}
