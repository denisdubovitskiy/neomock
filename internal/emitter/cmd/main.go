// Command main — отладочная утилита: рендерит мок фикстурного интерфейса
// в stdout (go run ./internal/emitter/cmd <InterfaceName>).
package main

import (
	"fmt"
	"os"

	"github.com/denisdubovitskiy/neomock/internal/emitter"
	"github.com/denisdubovitskiy/neomock/internal/loader"
)

const testdataPkg = "github.com/denisdubovitskiy/neomock/internal/loader/testdata/pkg"

func main() {
	name := "Server"
	if len(os.Args) > 1 {
		name = os.Args[1]
	}
	iface, err := loader.Load(
		loader.Config{
			PackagePath:   testdataPkg,
			InterfaceName: name,
		},
	)
	if err != nil {
		panic(err)
	}
	code, err := emitter.Render(iface, emitter.Options{Package: "mocks"})
	if err != nil {
		panic(err)
	}
	fmt.Print(code)
}
