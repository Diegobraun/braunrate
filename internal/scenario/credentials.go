package scenario

import (
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Diegobraun/braunrate/internal/protocol/transport"
)

// The same names the curl importer masks when it writes a file. A scenario goes
// to the repository, so a password written in it is a password published.
//
// A pergunta "este nome carrega credencial?" e a mesma que a mascara de saida
// faz, e responde-la em dois lugares foi como 'apiToken' passou a ser cortado na
// impressao e aceito literal no arquivo. Vale a resposta de transport, mais os
// nomes em portugues e os que so o cenario conhece.
var credentialNames = map[string]bool{
	"segredo": true, "pwd": true, "clientsecret": true,
	"secret_key": true, "access_key": true,
}

func credentialName(name string) bool {
	return credentialNames[strings.ToLower(name)] || transport.IsSecretName(name)
}

// Cabecalho que carrega credencial. O nome do campo nao diz "senha", mas o que
// vai depois dos dois pontos e um segredo do mesmo jeito.
var credentialHeaders = map[string]bool{
	"authorization": true, "proxy-authorization": true,
	"x-api-key": true, "api-key": true, "x-auth-token": true, "x-access-token": true,
}

// A recusa segue o valor, e nao o bloco: senha e senha em 'variables', no
// pedido de login e na linha de um cabecalho. A varredura roda sobre o
// documento cru, antes da expansao do ambiente, porque depois dela ${SENHA} e o
// segredo escrito no arquivo sao o mesmo texto.
//
// 'variables' e 'messaging' ficam de fora porque ja recusam com mensagem
// propria, e a delas ensina a forma certa daquele bloco.
func refuseLiteralCredentials(document *yaml.Node) error {
	for index := 0; index+1 < len(document.Content); index += 2 {
		key, value := document.Content[index], document.Content[index+1]
		if key.Value == "variables" || key.Value == "messaging" {
			continue
		}
		if err := sweepCredentials(value); err != nil {
			return err
		}
	}
	return nil
}

func sweepCredentials(node *yaml.Node) error {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.MappingNode {
		for index := 0; index+1 < len(node.Content); index += 2 {
			key, value := node.Content[index], node.Content[index+1]
			if err := refuseLiteralField(key.Value, value); err != nil {
				return err
			}
			if err := sweepCredentials(value); err != nil {
				return err
			}
		}
		return nil
	}
	for _, child := range node.Content {
		if err := sweepCredentials(child); err != nil {
			return err
		}
	}
	return nil
}

func refuseLiteralField(name string, node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode {
		return nil
	}
	lowered := strings.ToLower(name)
	switch {
	// Antes do nome de credencial porque 'Authorization' esta nas duas listas, e
	// ali o valor nao e so o segredo: 'Bearer ' vem na frente dele.
	case credentialHeaders[lowered]:
		if writtenSafely(node.Value) {
			return nil
		}
		return nodeError(node, "literal credential in the header %q: a credential never goes into the file, because the file goes into the repository.\n"+
			"    replace it with:  %s: \"Bearer ${TOKEN}\"\n"+
			"    and run with:  TOKEN=... braunrate execute scenario.yaml", name, name)
	case credentialName(lowered):
		if writtenSafely(node.Value) {
			return nil
		}
		environment := strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(lowered))
		return nodeError(node, "literal %s in the scenario: a credential never goes into the file, because the file goes into the repository.\n"+
			"    replace it with:  %s: \"${%s}\"\n"+
			"    and run with:  %s=... braunrate execute scenario.yaml",
			name, name, environment, environment)
	case lowered == "header":
		return refuseLiteralHeaderLine(node)
	}
	return nil
}

// 'auth.header' carrega a linha inteira, entao o nome do cabecalho esta do lado
// esquerdo do valor em vez de ser a chave do mapa.
func refuseLiteralHeaderLine(node *yaml.Node) error {
	name, credential, found := strings.Cut(node.Value, ":")
	if !found || !credentialHeaders[strings.ToLower(strings.TrimSpace(name))] || writtenSafely(credential) {
		return nil
	}
	return nodeError(node, "literal credential in 'header': a credential never goes into the file, because the file goes into the repository.\n"+
		"    replace it with:  header: \"%s: ${TOKEN}\"\n"+
		"    and run with:  TOKEN=... braunrate execute scenario.yaml", strings.TrimSpace(name))
}

// Referencia de ambiente e referencia de captura passam: nas duas o arquivo diz
// de onde o valor vem, e nao qual e.
func writtenSafely(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed == "" || strings.Contains(trimmed, "${") ||
		strings.HasPrefix(trimmed, "$.") ||
		strings.HasPrefix(trimmed, "header:") || strings.HasPrefix(trimmed, "cookie:")
}

func refuseLiteralVariable(name string, node *yaml.Node) error {
	if !credentialName(name) {
		return nil
	}
	value := strings.TrimSpace(node.Value)
	if value == "" || strings.Contains(value, "${") {
		return nil
	}
	environment := strings.ToUpper(name)
	return nodeError(node, "literal %s in 'variables': a credential never goes into the file, because the file goes into the repository.\n"+
		"    replace it with:  variables: { %s: \"${%s}\" }\n"+
		"    and run with:  %s=... braunrate execute scenario.yaml",
		name, name, environment, environment)
}
