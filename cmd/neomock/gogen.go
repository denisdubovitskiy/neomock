package main

import (
	"fmt"
	"os"

	"github.com/denisdubovitskiy/neomock/internal/config"
	"github.com/denisdubovitskiy/neomock/internal/gen"
)

// cmdGoGenerateA — режим А: go:generate-строка с флагами (§4.3).
// Приоритет: строка сильнее файла (строка инициировала запуск, §4.1).
func cmdGoGenerateA(args []string) int {
	f, err := config.ParseGenerateArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "neomock:", err)
		return 1
	}
	log := newLogger(f.LogLevel)
	cwd, err := os.Getwd()
	if err != nil {
		return exitError(err)
	}
	res, err := gen.Run(gen.ModeGenerate, cwd, gen.Options{
		ConfigPath: f.Config,
		GoGenA:     f,
	})
	if res != nil {
		reportWarnings(log, res.Warnings)
		for _, file := range res.Files {
			fmt.Printf("%-10s %s\n", file.Status, file.Path)
		}
	}
	if err != nil {
		return exitError(err)
	}
	return 0
}

// cmdGoGenerateB — режим Б: короткая строка `go tool neomock`,
// детали из секции пакета текущего каталога (§4.3).
func cmdGoGenerateB(_ []string) int {
	cwd, err := os.Getwd()
	if err != nil {
		return exitError(err)
	}
	log := newLogger("")
	res, err := gen.Run(gen.ModeGenerate, cwd, gen.Options{GoGenB: true})
	if res != nil {
		reportWarnings(log, res.Warnings)
		for _, file := range res.Files {
			fmt.Printf("%-10s %s\n", file.Status, file.Path)
		}
	}
	if err != nil {
		return exitError(err)
	}
	return 0
}
