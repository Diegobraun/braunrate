package scenario

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// The same names the curl importer masks when it writes a file. A scenario goes
// to the repository, so a password written in it is a password published.
var credentialNames = map[string]bool{
	"senha": true, "password": true, "pwd": true, "passwd": true,
	"secret": true, "segredo": true, "client_secret": true, "clientsecret": true,
	"token": true, "access_token": true, "refresh_token": true,
	"apikey": true, "api_key": true, "authorization": true,
	"secret_key": true, "access_key": true,
}

func refuseLiteralVariable(name string, node *yaml.Node) error {
	if !credentialNames[strings.ToLower(name)] {
		return nil
	}
	value := strings.TrimSpace(node.Value)
	if value == "" || strings.Contains(value, "${") {
		return nil
	}
	environment := strings.ToUpper(name)
	return nodeError(node, "%s literal em 'variaveis': credencial nunca vai para o arquivo, porque o arquivo vai para o repositorio.\n"+
		"    troque por:  variáveis: { %s: \"${%s}\" }\n"+
		"    e rode com:  %s=... braunrate execute cenario.yaml",
		name, name, environment, environment)
}
