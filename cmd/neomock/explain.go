package main

import (
	"fmt"
	"os"

	"github.com/denisdubovitskiy/neomock/internal/gen"
)

func cmdExplain(args []string) int {
	c, err := parseCommon(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if len(c.patterns) != 1 {
		fmt.Fprintln(
			os.Stderr,
			"neomock: explain требует ровно один аргумент <пакет.Интерфейс>",
		)
		return 2
	}
	pkgPath, ifaceName, err := gen.SplitIfaceID(c.patterns[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	cwd, err := os.Getwd()
	if err != nil {
		return exitError(err)
	}
	out, err := gen.Explain(cwd, c.configPath, pkgPath, ifaceName)
	if err != nil {
		return exitError(err)
	}
	fmt.Print(out)
	return 0
}
