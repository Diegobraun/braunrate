package cenario

import (
	"strings"
	"testing"

	_ "github.com/Diegobraun/braunrate/protocolo/graphql"
	_ "github.com/Diegobraun/braunrate/protocolo/http"
)

// Erro de cenario e quase sempre um erro de quem esta aprendendo o formato: a
// mensagem precisa mostrar a forma certa, nao so recusar a errada.
func TestMensagensDeErroMostramAFormaCerta(t *testing.T) {
	casos := []struct {
		nome     string
		conteudo string
		contem   []string
	}{
		{
			nome: "perfil escrito como mapa em vez de lista",
			conteudo: `nome: teste
alvo: http://127.0.0.1:8080
carga:
  perfis:
    patamar: { taxa: 300/s, durante: 5m }
cenario:
  - http: GET /pedidos/1
`,
			contem: []string{"perfis precisa ser uma lista", "- patamar: { taxa: 300/s, durante: 5m }"},
		},
		{
			nome: "duracao no formato de outra ferramenta",
			conteudo: `nome: teste
alvo: http://127.0.0.1:8080
carga:
  perfis:
    - patamar: { taxa: 300/s, durante: 5 minutos }
cenario:
  - http: GET /pedidos/1
`,
			contem: []string{"duracao invalida", "30s, 5m, 1h30m"},
		},
		{
			nome: "chave de carga com erro de digitacao",
			conteudo: `nome: teste
alvo: http://127.0.0.1:8080
carga:
  perfil:
    - patamar: { taxa: 300/s, durante: 5m }
cenario:
  - http: GET /pedidos/1
`,
			contem: []string{"chave desconhecida em carga", "perfis"},
		},
		{
			nome: "tipo de perfil inventado",
			conteudo: `nome: teste
alvo: http://127.0.0.1:8080
carga:
  perfis:
    - degrau: { taxa: 300/s, durante: 5m }
cenario:
  - http: GET /pedidos/1
`,
			contem: []string{"tipo de perfil desconhecido", "patamar"},
		},
		{
			nome: "autenticacao por token sem o bloco obter",
			conteudo: `nome: teste
alvo: http://127.0.0.1:8080
autenticacao:
  tipo: token
carga:
  perfis:
    - patamar: { taxa: 300/s, durante: 5m }
cenario:
  - http: GET /pedidos/1
`,
			contem: []string{"bloco 'obter'", "captura: { token: $.access_token }"},
		},
		{
			nome: "slo escrito como mapa em vez de lista",
			conteudo: `nome: teste
alvo: http://127.0.0.1:8080
carga:
  perfis:
    - patamar: { taxa: 300/s, durante: 5m }
cenario:
  - http: GET /pedidos/1
    nome: consultar pedido
slo:
  consultar pedido: { p95: < 150ms }
`,
			contem: []string{"slo precisa ser uma lista", "p95"},
		},
		{
			nome: "metrica de slo que nao existe",
			conteudo: `nome: teste
alvo: http://127.0.0.1:8080
carga:
  perfis:
    - patamar: { taxa: 300/s, durante: 5m }
cenario:
  - http: GET /pedidos/1
    nome: consultar pedido
slo:
  - consultar pedido: { media: < 150ms }
`,
			contem: []string{"metrica de slo desconhecida", "p95"},
		},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			_, err := Carregar([]byte(caso.conteudo))
			if err == nil {
				t.Fatal("esperava erro e o cenario carregou")
			}
			mensagem := err.Error()
			if !strings.Contains(mensagem, "linha") {
				t.Errorf("a mensagem precisa apontar a linha:\n%s", mensagem)
			}
			for _, trecho := range caso.contem {
				if !strings.Contains(mensagem, trecho) {
					t.Errorf("a mensagem nao mostra %q:\n%s", trecho, mensagem)
				}
			}
		})
	}
}
