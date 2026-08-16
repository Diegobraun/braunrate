package site

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const adrDirectory = "docs/adr"

// The ADRs are the reason the tool refuses things, and someone deciding whether
// to adopt it reads them before reading the reference. One line each, taken
// from the decision itself: a hand-written summary would be a second version of
// the decision, free to drift from the one that counts.
func DecisionsPage(repositoryRoot string) (Page, error) {
	directory := filepath.Join(repositoryRoot, adrDirectory)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return Page{}, fmt.Errorf("nao consegui ler %s: %w", directory, err)
	}
	var names []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".md") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	var markdown strings.Builder
	markdown.WriteString(`# Decisoes

Cada uma registra o que foi decidido, o que foi recusado e o criterio que
reabre a discussao. Os arquivos completos estao em ` + "`docs/adr`" + ` no
repositorio.

| # | decisao |
|---|---|
`)
	for _, name := range names {
		content, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			return Page{}, fmt.Errorf("nao consegui ler %s: %w", name, err)
		}
		title, found := firstHeading(string(content))
		if !found {
			return Page{}, fmt.Errorf("%s nao comeca com um titulo '# '", name)
		}
		link := "https://github.com/Diegobraun/braunrate/blob/main/docs/adr/" + name
		number, decision := splitTitle(title)
		fmt.Fprintf(&markdown, "| [%s](%s) | %s |\n", number, link, cell(decision))
	}
	return Page{Slug: "decisoes", Title: "Decisoes", Markdown: markdown.String()}, nil
}

// The ADR title already is the decision in one line — that is what an ADR title
// is for. Summarising the body here would create a second version of the
// decision, free to drift from the one that counts.
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
