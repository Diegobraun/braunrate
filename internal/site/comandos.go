package site

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

// A tabela de comandos e a mesma no markdown e no site; o que muda e a forma.
// Os cartoes saem dela e da primeira linha de exemplo de cada secao, entao nao
// existe segunda lista de comandos para alguem esquecer de atualizar.
var (
	indexRow      = regexp.MustCompile(`(?m)^\| \[` + "`" + `([^` + "`" + `]+)` + "`" + `\]\(#([^)]+)\) \| ([^|]+) \|\s*$`)
	indexTable    = regexp.MustCompile(`(?m)^\| Comando \| Para quê \|\n\|[-| ]+\|\n(\|.*\|\n)+`)
	commandSample = regexp.MustCompile("(?s)\n## `%s`\n+```[a-z]*\n([^\n]+)\n")
)

func commandCards(markdown string) (string, string, bool) {
	table := indexTable.FindString(markdown)
	if table == "" {
		return "", markdown, false
	}
	rows := indexRow.FindAllStringSubmatch(table, -1)
	if len(rows) == 0 {
		return "", markdown, false
	}

	var written strings.Builder
	written.WriteString("<div class=\"cartoes\">\n")
	for _, row := range rows {
		name, anchor, description := row[1], row[2], strings.TrimSpace(row[3])
		sample := firstSample(markdown, name)
		fmt.Fprintf(&written, `<a class="cartao" href="#%s"><p class="nome"><code>%s</code></p>`+
			`<p class="para-que">%s</p><p class="exemplo"><code>%s</code></p></a>`+"\n",
			html.EscapeString(anchor), html.EscapeString(name),
			html.EscapeString(description), html.EscapeString(sample))
	}
	written.WriteString("</div>\n")
	return written.String(), strings.Replace(markdown, table, "\n"+cardsMarker+"\n\n", 1), true
}

// Paragrafo comum em vez de HTML cru: o goldmark so deixa HTML passar com a
// opcao insegura ligada, e ligar isso para um marcador seria abrir o caminho
// para HTML dentro de guia.
const cardsMarker = "CARTOES-DE-COMANDO"

// A grade sobe para logo abaixo do titulo: ela e o indice da pagina, e indice
// depois de um exemplo de erro obriga a rolar para achar o comando procurado.
// A ordem do markdown continua a mesma; o que muda e onde a grade aparece.
func placeCards(rendered, cards string) string {
	rendered = strings.Replace(rendered, "<p>"+cardsMarker+"</p>\n", "", 1)
	if end := strings.Index(rendered, "</h1>"); end >= 0 {
		cut := end + len("</h1>\n")
		return rendered[:cut] + cards + rendered[cut:]
	}
	return cards + rendered
}

func firstSample(markdown, command string) string {
	pattern := regexp.MustCompile(fmt.Sprintf(commandSample.String(), regexp.QuoteMeta(command)))
	match := pattern.FindStringSubmatch(markdown)
	if match == nil {
		return "braunrate " + command
	}
	return strings.TrimPrefix(strings.TrimSpace(match[1]), "$ ")
}
