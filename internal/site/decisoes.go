package site

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const adrDirectory = "docs/adr"

func DecisionsPage(repositoryRoot string) (Page, error) {
	directory := filepath.Join(repositoryRoot, adrDirectory)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return Page{}, fmt.Errorf("não consegui ler %s: %w", directory, err)
	}
	var names []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".md") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	var markdown strings.Builder
	markdown.WriteString(`# Decisões

Cada uma registra o que foi decidido, o que foi recusado e o critério que
reabre a discussão. Os arquivos completos estão em ` + "`docs/adr`" + ` no
repositório.

| # | decisão |
|---|---|
`)
	for _, name := range names {
		content, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			return Page{}, fmt.Errorf("não consegui ler %s: %w", name, err)
		}
		title, found := firstHeading(string(content))
		if !found {
			return Page{}, fmt.Errorf("%s não começa com um título '# '", name)
		}
		link := "https://github.com/Diegobraun/braunrate/blob/main/docs/adr/" + name
		number, decision := splitTitle(title)
		fmt.Fprintf(&markdown, "| [%s](%s) | %s |\n", number, link, cell(decision))
	}
	return Page{Slug: "decisoes", Title: "Decisões", Section: "Referência",
		Summary:  "As decisões de arquitetura registradas, uma linha cada.",
		Markdown: markdown.String(), Source: adrDirectory}, nil
}

// O titulo do ADR ja e a decisao em uma linha. Resumir o corpo aqui criaria uma
// segunda versao dela, livre para divergir da que vale.
func splitTitle(title string) (string, string) {
	rest, found := strings.CutPrefix(title, "ADR ")
	if !found {
		return title, title
	}
	number, decision, found := strings.Cut(rest, " — ")
	if !found {
		number, decision, _ = strings.Cut(rest, " - ")
	}
	return strings.TrimSpace(number), strings.TrimSpace(decision)
}
