package importador

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/cenario"
	_ "github.com/Diegobraun/braunrate/protocolo/http"
)

func TestImportaCurlDeUmaLinhaEGeraCenarioQueOParserAceita(t *testing.T) {
	importacao, err := DeCurl(`curl https://api.exemplo.com/saude`)
	if err != nil {
		t.Fatalf("nao importou: %v", err)
	}
	lido, err := cenario.Carregar([]byte(importacao.YAML))
	if err != nil {
		t.Fatalf("o cenario gerado nao carrega: %v\n%s", err, importacao.YAML)
	}
	if len(lido.Passos) != 1 {
		t.Fatalf("esperava 1 passo, veio %d", len(lido.Passos))
	}
	if lido.Passos[0].Nome != "get saude" {
		t.Errorf("nome do passo veio %q", lido.Passos[0].Nome)
	}
}

func TestImportaCurlColadoDoNavegadorComQuebraDeLinha(t *testing.T) {
	comando := `curl 'https://api.exemplo.com/v2/faturas/887766/pagar' \
  -X POST \
  -H 'Content-Type: application/json' \
  --data-raw '{"valor": 199.90, "metodo": "pix"}'`

	importacao, err := DeCurl(comando)
	if err != nil {
		t.Fatalf("nao importou: %v", err)
	}
	if _, err := cenario.Carregar([]byte(importacao.YAML)); err != nil {
		t.Fatalf("o cenario gerado nao carrega: %v\n%s", err, importacao.YAML)
	}
	if !strings.Contains(importacao.YAML, `corpo: '{"valor": 199.90, "metodo": "pix"}'`) {
		t.Errorf("o corpo nao sobreviveu a separacao:\n%s", importacao.YAML)
	}
	if !strings.Contains(importacao.YAML, "metodo: POST") {
		t.Errorf("perdeu o metodo declarado:\n%s", importacao.YAML)
	}
	if !strings.Contains(importacao.YAML, "nome: post faturas pagar") {
		t.Errorf("o identificador entrou no nome do passo, o que geraria uma linha de relatorio por requisicao:\n%s", importacao.YAML)
	}
}

func TestTokenDoCurlNaoVaiParaOArquivo(t *testing.T) {
	importacao, err := DeCurl(`curl https://api.exemplo.com/pedidos -H "Authorization: Bearer abc.def.ghi" -H "X-API-Key: chave-secreta"`)
	if err != nil {
		t.Fatalf("nao importou: %v", err)
	}
	for _, segredo := range []string{"abc.def.ghi", "chave-secreta"} {
		if strings.Contains(importacao.YAML, segredo) {
			t.Errorf("o segredo %q ficou no YAML gerado", segredo)
		}
	}
	if !strings.Contains(importacao.YAML, "token: ${TOKEN}") || !strings.Contains(importacao.YAML, "api_key: ${API_KEY}") {
		t.Errorf("o segredo nao virou variavel de ambiente:\n%s", importacao.YAML)
	}
	if len(importacao.Avisos) < 2 {
		t.Errorf("importacao com dois segredos precisa avisar sobre os dois, veio %d aviso(s)", len(importacao.Avisos))
	}
	if _, err := cenario.Carregar([]byte(importacao.YAML)); err != nil {
		t.Fatalf("o cenario gerado nao carrega: %v\n%s", err, importacao.YAML)
	}
}

func TestMetodoViraPostQuandoOCurlTemCorpoSemMenosX(t *testing.T) {
	importacao, err := DeCurl(`curl https://api.exemplo.com/pedidos -d '{"a":1}'`)
	if err != nil {
		t.Fatalf("nao importou: %v", err)
	}
	if !strings.Contains(importacao.YAML, "metodo: POST") {
		t.Errorf("curl com corpo e POST no curl tambem:\n%s", importacao.YAML)
	}
}

func TestAvisaQueValorFixoNoCaminhoDeixaONumeroOtimista(t *testing.T) {
	importacao, err := DeCurl(`curl https://api.exemplo.com/pedidos/9912`)
	if err != nil {
		t.Fatalf("nao importou: %v", err)
	}
	achou := false
	for _, aviso := range importacao.Avisos {
		if strings.Contains(aviso, "cache") {
			achou = true
		}
	}
	if !achou {
		t.Errorf("importacao com id fixo precisa avisar sobre cache, veio: %v", importacao.Avisos)
	}
}

func TestErroDeCurlSemURLDizOQueFazer(t *testing.T) {
	_, err := DeCurl(`curl -X POST -H "Accept: application/json"`)
	if err == nil {
		t.Fatal("curl sem URL precisa falhar")
	}
	if !strings.Contains(err.Error(), "curl https://exemplo/pedidos") {
		t.Errorf("a mensagem precisa mostrar a forma certa, veio: %v", err)
	}
}

func TestErroDeAspasAbertasDizOQueFazer(t *testing.T) {
	_, err := DeCurl(`curl 'https://api.exemplo.com/pedidos`)
	if err == nil {
		t.Fatal("aspas abertas precisam falhar")
	}
	if !strings.Contains(err.Error(), "aspas") {
		t.Errorf("a mensagem precisa falar de aspas, veio: %v", err)
	}
}

func TestCenarioGeradoApontaParaOSchemaPublicado(t *testing.T) {
	importacao, err := DeCurl(`curl https://api.exemplo.com/saude`)
	if err != nil {
		t.Fatalf("nao importou: %v", err)
	}
	if !strings.HasPrefix(importacao.YAML, "# yaml-language-server: $schema=") {
		t.Errorf("o arquivo gerado precisa dar autocompletar no editor de quem recebeu:\n%s", importacao.YAML)
	}
}
