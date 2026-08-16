package importer

import (
	"strings"
	"testing"

	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

func TestImportsSingleLineCurlIntoParsableScenario(t *testing.T) {
	importResult, err := FromCurl(`curl https://api.exemplo.com/saude`)
	if err != nil {
		t.Fatalf("nao importou: %v", err)
	}
	read, err := scenario.Parse([]byte(importResult.YAML))
	if err != nil {
		t.Fatalf("o cenario gerado nao carrega: %v\n%s", err, importResult.YAML)
	}
	if len(read.Steps) != 1 {
		t.Fatalf("esperava 1 passo, veio %d", len(read.Steps))
	}
	if read.Steps[0].Name != "get saude" {
		t.Errorf("nome do passo veio %q", read.Steps[0].Name)
	}
}

func TestImportsCurlPastedFromBrowserWithLineBreaks(t *testing.T) {
	command := `curl 'https://api.exemplo.com/v2/faturas/887766/pagar' \
  -X POST \
  -H 'Content-Type: application/json' \
  --data-raw '{"valor": 199.90, "metodo": "pix"}'`

	importResult, err := FromCurl(command)
	if err != nil {
		t.Fatalf("nao importou: %v", err)
	}
	if _, err := scenario.Parse([]byte(importResult.YAML)); err != nil {
		t.Fatalf("o cenario gerado nao carrega: %v\n%s", err, importResult.YAML)
	}
	if !strings.Contains(importResult.YAML, `corpo: '{"valor": 199.90, "metodo": "pix"}'`) {
		t.Errorf("o corpo nao sobreviveu a separacao:\n%s", importResult.YAML)
	}
	if !strings.Contains(importResult.YAML, "metodo: POST") {
		t.Errorf("perdeu o metodo declarado:\n%s", importResult.YAML)
	}
	if !strings.Contains(importResult.YAML, "nome: post faturas pagar") {
		t.Errorf("o identificador entrou no nome do passo, o que geraria uma linha de relatorio por requisicao:\n%s", importResult.YAML)
	}
}

func TestCurlTokenNeverReachesFile(t *testing.T) {
	importResult, err := FromCurl(`curl https://api.exemplo.com/pedidos -H "Authorization: Bearer abc.def.ghi" -H "X-API-Key: chave-secreta"`)
	if err != nil {
		t.Fatalf("nao importou: %v", err)
	}
	for _, secret := range []string{"abc.def.ghi", "chave-secreta"} {
		if strings.Contains(importResult.YAML, secret) {
			t.Errorf("o segredo %q ficou no YAML gerado", secret)
		}
	}
	if !strings.Contains(importResult.YAML, "token: ${TOKEN}") || !strings.Contains(importResult.YAML, "api_key: ${API_KEY}") {
		t.Errorf("o segredo nao virou variavel de ambiente:\n%s", importResult.YAML)
	}
	if len(importResult.Warnings) < 2 {
		t.Errorf("importacao com dois segredos precisa avisar sobre os dois, veio %d aviso(s)", len(importResult.Warnings))
	}
	if _, err := scenario.Parse([]byte(importResult.YAML)); err != nil {
		t.Fatalf("o cenario gerado nao carrega: %v\n%s", err, importResult.YAML)
	}
}

func TestMethodBecomesPostWhenCurlHasBodyWithoutX(t *testing.T) {
	importResult, err := FromCurl(`curl https://api.exemplo.com/pedidos -d '{"a":1}'`)
	if err != nil {
		t.Fatalf("nao importou: %v", err)
	}
	if !strings.Contains(importResult.YAML, "metodo: POST") {
		t.Errorf("curl com corpo e POST no curl tambem:\n%s", importResult.YAML)
	}
}

func TestWarnsFixedPathValueMakesNumberOptimistic(t *testing.T) {
	importResult, err := FromCurl(`curl https://api.exemplo.com/pedidos/9912`)
	if err != nil {
		t.Fatalf("nao importou: %v", err)
	}
	found := false
	for _, warning := range importResult.Warnings {
		if strings.Contains(warning, "cache") {
			found = true
		}
	}
	if !found {
		t.Errorf("importacao com id fixo precisa avisar sobre cache, veio: %v", importResult.Warnings)
	}
}

func TestCurlWithoutURLSaysWhatToDo(t *testing.T) {
	_, err := FromCurl(`curl -X POST -H "Accept: application/json"`)
	if err == nil {
		t.Fatal("curl sem URL precisa falhar")
	}
	if !strings.Contains(err.Error(), "curl https://exemplo/pedidos") {
		t.Errorf("a mensagem precisa mostrar a forma certa, veio: %v", err)
	}
}

func TestUnclosedQuoteErrorSaysWhatToDo(t *testing.T) {
	_, err := FromCurl(`curl 'https://api.exemplo.com/pedidos`)
	if err == nil {
		t.Fatal("aspas abertas precisam falhar")
	}
	if !strings.Contains(err.Error(), "aspas") {
		t.Errorf("a mensagem precisa falar de aspas, veio: %v", err)
	}
}

func TestGeneratedScenarioPointsToPublishedSchema(t *testing.T) {
	importResult, err := FromCurl(`curl https://api.exemplo.com/saude`)
	if err != nil {
		t.Fatalf("nao importou: %v", err)
	}
	if !strings.HasPrefix(importResult.YAML, "# yaml-language-server: $schema=") {
		t.Errorf("o arquivo gerado precisa dar autocompletar no editor de quem recebeu:\n%s", importResult.YAML)
	}
}

// A recorded or pasted login carries the password in plain text, and the
// generated file goes to the repository. The header rule had this covered and
// the body did not.
func TestPasswordInTheBodyNeverReachesTheFile(t *testing.T) {
	result, err := FromCurl(`curl -X POST https://api.exemplo/auth -H 'Content-Type: application/json' -d '{"usuario":"ana","senha":"p4ssw0rd-real"}'`)
	if err != nil {
		t.Fatalf("importar falhou: %v", err)
	}

	if strings.Contains(result.YAML, "p4ssw0rd-real") {
		t.Fatalf("a senha foi parar no cenario:\n%s", result.YAML)
	}
	if !strings.Contains(result.YAML, `"senha": "${senha}"`) {
		t.Fatalf("a senha nao virou referencia:\n%s", result.YAML)
	}
	if !strings.Contains(result.YAML, "senha: ${SENHA}") {
		t.Fatalf("faltou declarar a variavel de ambiente:\n%s", result.YAML)
	}
	found := false
	for _, warning := range result.Warnings {
		if strings.Contains(warning, "SENHA=") {
			found = true
		}
	}
	if !found {
		t.Fatalf("o corte foi silencioso; avisos: %v", result.Warnings)
	}
}
