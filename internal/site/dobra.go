package site

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

// A dobra e declarada no proprio markdown da pagina inicial, e nao montada no
// gerador: o texto que a pessoa le primeiro tem que viver onde o resto do texto
// vive, e chegar por pull request como os guias chegam.
type Hero struct {
	Lema    string
	Resumo  string
	Comando string
	Ficha   []string
	Acoes   []Action
	Prova   string
	Lados   []Side
	Saldo   string
}

type Action struct {
	Label string
	Href  string
}

type Side struct {
	Nome   string
	Numero string
	Frase  string
}

var heroBlock = regexp.MustCompile("(?s)```dobra\n(.*?)```\n?")

func extractHero(markdown string) (*Hero, string) {
	match := heroBlock.FindStringSubmatch(markdown)
	if match == nil {
		return nil, markdown
	}
	hero := &Hero{}
	for _, line := range strings.Split(match[1], "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "lema":
			hero.Lema = value
		case "resumo":
			hero.Resumo = value
		case "comando":
			hero.Comando = value
		case "ficha":
			hero.Ficha = fields(value)
		case "acao":
			if parts := fields(value); len(parts) == 2 {
				hero.Acoes = append(hero.Acoes, Action{Label: parts[0], Href: parts[1]})
			}
		case "prova":
			hero.Prova = value
		case "lado":
			if parts := fields(value); len(parts) == 3 {
				hero.Lados = append(hero.Lados, Side{Nome: parts[0], Numero: parts[1], Frase: parts[2]})
			}
		case "saldo":
			hero.Saldo = value
		}
	}
	return hero, heroBlock.ReplaceAllString(markdown, "")
}

func fields(value string) []string {
	parts := strings.Split(value, "|")
	for index, part := range parts {
		parts[index] = strings.TrimSpace(part)
	}
	return parts
}

func (hero *Hero) render() string {
	var written strings.Builder
	written.WriteString("    <section class=\"dobra\">\n")
	written.WriteString("      <h1>braunrate</h1>\n")
	fmt.Fprintf(&written, "      <p class=\"lema\">%s</p>\n", html.EscapeString(hero.Lema))
	fmt.Fprintf(&written, "      <p class=\"resumo\">%s</p>\n", html.EscapeString(hero.Resumo))

	written.WriteString("      <div class=\"chamado\">\n")
	fmt.Fprintf(&written, "        <code class=\"comando\" id=\"comando-da-dobra\">%s</code>"+
		"<button type=\"button\" class=\"copiar-comando\" data-alvo=\"comando-da-dobra\">copiar</button>\n",
		html.EscapeString(hero.Comando))
	for index, acao := range hero.Acoes {
		class := "secundario"
		if index == 0 {
			class = "secundario primeiro"
		}
		fmt.Fprintf(&written, "        <a class=%q href=%q>%s</a>\n",
			class, html.EscapeString(acao.Href), html.EscapeString(acao.Label))
	}
	written.WriteString("      </div>\n")

	if len(hero.Ficha) > 0 {
		written.WriteString("      <p class=\"ficha\">")
		for index, item := range hero.Ficha {
			if index > 0 {
				written.WriteString("<span aria-hidden=\"true\"> · </span>")
			}
			written.WriteString(html.EscapeString(item))
		}
		written.WriteString("</p>\n")
	}

	if len(hero.Lados) == 2 {
		written.WriteString("      <figure class=\"prova\">\n")
		fmt.Fprintf(&written, "        <figcaption>%s</figcaption>\n", html.EscapeString(hero.Prova))
		written.WriteString("        <div class=\"lados\">\n")
		for index, lado := range hero.Lados {
			class := "lado"
			if index == 1 {
				class = "lado nosso"
			}
			fmt.Fprintf(&written, "          <div class=%q><p class=\"nome\">%s</p>"+
				"<p class=\"numero\">%s</p><p class=\"frase\">%s</p></div>\n",
				class, html.EscapeString(lado.Nome), html.EscapeString(lado.Numero), html.EscapeString(lado.Frase))
		}
		written.WriteString("        </div>\n")
		fmt.Fprintf(&written, "        <p class=\"saldo\">%s</p>\n", html.EscapeString(hero.Saldo))
		written.WriteString("      </figure>\n")
	}
	written.WriteString("    </section>\n")
	return written.String()
}
