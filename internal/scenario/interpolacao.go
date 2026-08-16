package scenario

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// A regra, uma so: **${VARIAVEL} do ambiente vale em qualquer campo escalar do
// cenario**, resolvido quando o arquivo e lido.
//
// Antes disso a interpolacao era campo a campo — `alvo` aceitava, `taxa` nao,
// `semente` passou a aceitar num dia e `topico` nunca aceitou. Inconsistencia de
// formato e o que se descobre em cinco minutos e nao se esquece, e a resposta
// certa nao e acrescentar um campo de cada vez.
//
// O que continua **nao** sendo resolvido aqui: `${assinantes.id}` e
// `${faturaId}`, que mudam por iteracao e sao resolvidos pelo motor. A diferenca
// e a caixa do nome, que ja era a convencao do projeto (ver environmentName):
// MAIUSCULA vem do ambiente, minuscula vem do arquivo ou da captura.
func expandEnvironment(node *yaml.Node) {
	if node == nil {
		return
	}
	if node.Kind == yaml.MappingNode {
		for index := 0; index+1 < len(node.Content); index += 2 {
			key, value := node.Content[index], node.Content[index+1]
			if keepsRawText(key.Value) {
				continue
			}
			expandEnvironment(value)
		}
		return
	}
	if node.Kind == yaml.ScalarNode {
		node.Value = expandDefinedEnvironment(node.Value)
		return
	}
	for _, child := range node.Content {
		expandEnvironment(child)
	}
}

// Duas excecoes, e as duas porque o **texto cru do campo faz parte do que ele
// significa** — nao porque a regra vale menos ali.
//
//   - credencial: a recusa de segredo literal confere se a pessoa escreveu
//     ${VARIAVEL} ou o segredo. Resolver antes faria a ferramenta recusar um
//     arquivo certo e ensinar o contrario do que ela pede.
//   - semente: o relatorio publica de qual variavel a semente veio, para que a
//     execucao possa ser repetida. Resolver antes apaga a origem.
//
// Nas duas, quem le o campo faz a propria expansao — ReadSeed e readVars —, e
// so ali o texto cru some.
func keepsRawText(key string) bool {
	return credentialNames[strings.ToLower(key)] || key == "semente"
}

// Referencia de ambiente que nao esta definida e nao tem padrao fica como esta,
// em vez de virar vazio em silencio. Vazio em silencio e como um alvo ausente
// virava "o cenario precisa de um alvo", sem dizer qual variavel faltou.
func expandDefinedEnvironment(text string) string {
	if !strings.Contains(text, "${") {
		return text
	}
	return varPattern.ReplaceAllStringFunc(text, func(occurrence string) string {
		parts := varPattern.FindStringSubmatch(occurrence)
		name, fallback := parts[1], parts[2]
		if !environmentName.MatchString(name) {
			return occurrence
		}
		if value, present := os.LookupEnv(name); present {
			return value
		}
		if strings.Contains(occurrence, ":-") {
			return fallback
		}
		return occurrence
	})
}

// Explica o erro de campo quando o que sobrou nele foi uma referencia que o
// ambiente nao tinha: sem esta frase, "taxa invalida: ${TAXA}/s" manda a pessoa
// procurar erro de sintaxe onde falta uma variavel.
func missingEnvironmentHint(node *yaml.Node) string {
	if node == nil || !strings.Contains(node.Value, "${") {
		return ""
	}
	missing := UnresolvedEnvironment(node.Value)
	if len(missing) == 0 {
		return ""
	}
	if len(missing) == 1 {
		return fmt.Sprintf("\n    a variável de ambiente %s não está definida, então este campo ficou com a referência crua.\n"+
			"    rode com %s=... , ou declare um padrão no arquivo: ${%s:-valor}",
			missing[0], missing[0], missing[0])
	}
	return fmt.Sprintf("\n    estas variáveis de ambiente não estão definidas: %s.\n"+
		"    rode com elas no ambiente, ou declare um padrão no arquivo: ${NOME:-valor}",
		strings.Join(missing, ", "))
}

// interpolateKnown resolve o que da para resolver e **deixa como esta o que nao
// da**. A diferenca importa: apagar a referencia transformava
// "alvo: ${ALVO}" sem ALVO no ambiente em "o cenario precisa de um alvo", uma
// frase que manda procurar um campo que a pessoa escreveu.
func interpolateKnown(text string, vars map[string]string) string {
	if !strings.Contains(text, "${") {
		return text
	}
	return varPattern.ReplaceAllStringFunc(text, func(occurrence string) string {
		parts := varPattern.FindStringSubmatch(occurrence)
		name, fallback := parts[1], parts[2]
		if value, declared := vars[name]; declared {
			return value
		}
		if value, present := os.LookupEnv(name); present {
			return value
		}
		if strings.Contains(occurrence, ":-") {
			return fallback
		}
		return occurrence
	})
}

// UnresolvedEnvironment lista as variaveis de ambiente que o texto ainda
// referencia e o ambiente nao tem.
func UnresolvedEnvironment(text string) []string {
	var missing []string
	for _, used := range referencesIn(text) {
		if used.hasDefault || !environmentName.MatchString(used.name) || defined(used.name) {
			continue
		}
		missing = append(missing, used.name)
	}
	return missing
}
