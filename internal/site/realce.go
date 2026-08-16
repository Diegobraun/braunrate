package site

import (
	"regexp"
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
)

// O destaque de sintaxe sai em classe, e nao em cor dentro da tag: com cor
// embutida o bloco so pode ter uma paleta, e ela ficava escura no tema claro.
// As duas paletas sao geradas aqui, na build, para nao virar folha de terceiro.
func highlightStyles() (string, error) {
	claro, escuro := codeBackgrounds()
	light, err := paletteCSS("github", "", claro)
	if err != nil {
		return "", err
	}
	dark, err := paletteCSS("github-dark", ":root:not([data-tema=\"claro\"]) ", escuro)
	if err != nil {
		return "", err
	}
	forced, err := paletteCSS("github-dark", ":root[data-tema=\"escuro\"] ", escuro)
	if err != nil {
		return "", err
	}
	return "\n/* destaque de sintaxe, gerado na build */\n" + light +
		"@media (prefers-color-scheme: dark) {\n" + dark + "}\n" + forced, nil
}

func paletteCSS(name, scope, background string) (string, error) {
	var written strings.Builder
	formatter := chromahtml.New(chromahtml.WithClasses(true))
	if err := formatter.WriteCSS(&written, styles.Get(name)); err != nil {
		return "", err
	}
	css := withReadableColors(withoutBackground(written.String()), background)
	return prefixSelectors(css, scope), nil
}

// O fundo do bloco e do tema do site, e nao da paleta: o cinza que o chroma traz
// nao e o mesmo dos outros cartoes da pagina, e a diferenca aparece.
var background = regexp.MustCompile(`background-color: #[0-9a-fA-F]+;? *`)

func withoutBackground(css string) string {
	var written strings.Builder
	for _, line := range strings.Split(css, "\n") {
		if strings.HasPrefix(line, "/* Background */") || strings.HasPrefix(line, "/* PreWrapper */") {
			line = background.ReplaceAllString(line, "")
		}
		written.WriteString(line + "\n")
	}
	return strings.TrimSuffix(written.String(), "\n")
}

// A folha do chroma sai uma regra por linha, com um comentario na frente. O
// escopo entra depois do comentario e antes de cada seletor da lista.
func prefixSelectors(css, scope string) string {
	if scope == "" {
		return css
	}
	var written strings.Builder
	for _, line := range strings.Split(css, "\n") {
		rule := line
		comment := ""
		if end := strings.LastIndex(line, "*/"); end >= 0 {
			comment, rule = line[:end+2]+" ", line[end+2:]
		}
		selectors, body, found := strings.Cut(rule, "{")
		if !found {
			written.WriteString(line + "\n")
			continue
		}
		var scoped []string
		for _, selector := range strings.Split(selectors, ",") {
			selector = strings.TrimSpace(selector)
			if selector == "" {
				continue
			}
			scoped = append(scoped, scope+selector)
		}
		written.WriteString(comment + strings.Join(scoped, ", ") + " {" + body + "\n")
	}
	return written.String()
}
