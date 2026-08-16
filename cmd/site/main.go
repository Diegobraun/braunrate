// Command site generates the published documentation from this repository.
// It is a build tool, not part of the braunrate binary.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Diegobraun/braunrate/internal/build"
	"github.com/Diegobraun/braunrate/internal/site"
)

func main() {
	root := flag.String("root", ".", "raiz do repositório")
	destination := flag.String("out", "site", "diretório onde o site é gravado")
	flag.Parse()

	if err := site.Build(*root, *destination, build.Version); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "site em %s\n", *destination)
}
