package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Diegobraun/braunrate/internal/scenario"
)

func main() {
	for _, path := range os.Args[1:] {
		content, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}
		text := string(content)
		var out strings.Builder
		rest := text
		for {
			start := strings.Index(rest, "```yaml trecho\n")
			if start < 0 {
				out.WriteString(rest)
				break
			}
			out.WriteString(rest[:start+len("```yaml trecho\n")])
			rest = rest[start+len("```yaml trecho\n"):]
			end := strings.Index(rest, "```")
			if end < 0 {
				out.WriteString(rest)
				break
			}
			block := rest[:end]
			migrated, changes, err := scenario.Migrate([]byte(block))
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: bloco nao migrou: %v\n", path, err)
				out.WriteString(block)
			} else {
				if len(changes) > 0 {
					fmt.Printf("%s: %d mudanca(s)\n", path, len(changes))
				}
				out.Write(migrated)
			}
			rest = rest[end:]
		}
		if err := os.WriteFile(path, []byte(out.String()), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
}
