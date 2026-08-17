package site

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const adrDirectory = "docs/adr"

func DecisionsPage(repositoryRoot string, language Language) (Page, error) {
	directory := filepath.Join(repositoryRoot, adrDirectory)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return Page{}, fmt.Errorf("could not read %s: %w", directory, err)
	}
	var names []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".md") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	text := language.Text
	var markdown strings.Builder
	fmt.Fprintf(&markdown, "# %s\n\n%s\n\n%s\n|---|---|\n",
		text.DecisionsTitle, text.DecisionsIntro, text.DecisionsColumns)
	for _, name := range names {
		content, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			return Page{}, fmt.Errorf("could not read %s: %w", name, err)
		}
		title, found := firstHeading(string(content))
		if !found {
			return Page{}, fmt.Errorf("%s does not start with a '# ' heading", name)
		}
		link := "https://github.com/Diegobraun/braunrate/blob/main/docs/adr/" + name
		number, decision := splitTitle(title)
		fmt.Fprintf(&markdown, "| [%s](%s) | %s |\n", number, link, cell(decision))
	}
	return Page{Slug: "decisions", Title: text.DecisionsTitle, Section: text.Sections["reference"],
		Summary:  text.DecisionsSummary,
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
