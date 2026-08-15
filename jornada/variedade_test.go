package jornada_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/Diegobraun/braunrate/cenario"
	"github.com/Diegobraun/braunrate/metrica"
	"github.com/Diegobraun/braunrate/motor"
	_ "github.com/Diegobraun/braunrate/protocolo/graphql"
	_ "github.com/Diegobraun/braunrate/protocolo/http"
)

// Um cenario que exercita todos os lugares por onde um valor variavel sai:
// caminho, corpo, cabecalho e variavel de GraphQL. O bug do dado congelado nao
// foi um defeito isolado da autenticacao — foi uma classe de falha que a suite
// nao verificava, porque contagem, latencia e erro continuam bonitos quando a
// carga inteira usa o mesmo valor.
const cenarioComVariedade = `
nome: Variedade
alvo: %s

autenticacao:
  tipo: token
  obter:
    http: { metodo: POST, caminho: /auth/token, corpo: { usuario: ana } }
    captura: { token: $.access_token }

dados:
  assinantes:
    arquivo: assinantes.csv
    consumo: circular

carga:
  perfis:
    - constante: { taxa: 60/s, durante: 1s }

cenario:
  - http: GET /pedidos/${assinantes.id}
    nome: caminho

  - nome: corpo
    http:
      metodo: POST
      caminho: /faturas/pagar
      corpo: { assinante: "${assinantes.id}", regiao: "${assinantes.regiao}" }

  - nome: cabecalho
    http:
      metodo: GET
      caminho: /eco
      cabecalhos: { X-Assinante: "${assinantes.id}" }

  - graphql:
      consulta: |
        query ConsultarPedido($id: ID!) { pedido(id: $id) { status } }
      variaveis: { id: "${assinantes.id}" }
`

type coletorDeValores struct {
	mutex  sync.Mutex
	vistos map[string]map[string]struct{}
}

func (c *coletorDeValores) anotar(onde, valor string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.vistos[onde] == nil {
		c.vistos[onde] = map[string]struct{}{}
	}
	c.vistos[onde][valor] = struct{}{}
}

func (c *coletorDeValores) distintos(onde string) []string {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	valores := make([]string, 0, len(c.vistos[onde]))
	for valor := range c.vistos[onde] {
		valores = append(valores, valor)
	}
	sort.Strings(valores)
	return valores
}

func TestTodoValorDeclaradoChegaAoAlvo(t *testing.T) {
	recebidos := &coletorDeValores{vistos: map[string]map[string]struct{}{}}

	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		corpo, _ := io.ReadAll(r.Body)

		switch {
		case r.URL.Path == "/auth/token":
			fmt.Fprint(w, `{"access_token":"token-de-teste"}`)
			return
		case strings.HasPrefix(r.URL.Path, "/pedidos/"):
			recebidos.anotar("caminho", strings.TrimPrefix(r.URL.Path, "/pedidos/"))
		case r.URL.Path == "/faturas/pagar":
			var enviado struct {
				Assinante string `json:"assinante"`
				Regiao    string `json:"regiao"`
			}
			_ = json.Unmarshal(corpo, &enviado)
			recebidos.anotar("corpo", enviado.Assinante)
			recebidos.anotar("corpo.regiao", enviado.Regiao)
		case r.URL.Path == "/eco":
			recebidos.anotar("cabecalho", r.Header.Get("X-Assinante"))
		case r.URL.Path == "/graphql":
			var enviado struct {
				Variaveis map[string]any `json:"variables"`
			}
			_ = json.Unmarshal(corpo, &enviado)
			recebidos.anotar("graphql", fmt.Sprint(enviado.Variaveis["id"]))
		}
		fmt.Fprint(w, `{"data":{"pedido":{"status":"ABERTO"}},"status":"ABERTO"}`)
	}))
	t.Cleanup(servidor.Close)

	documento := executarCenarioDeVariedade(t, servidor.URL,
		"id,regiao\n1001,sul\n1002,norte\n1003,leste\n")

	if documento.Global.Erros != 0 {
		t.Fatalf("esperava zero erro, obtive %d: %+v", documento.Global.Erros, documento.Passos)
	}

	esperados := []string{"1001", "1002", "1003"}
	for _, onde := range []string{"caminho", "corpo", "cabecalho", "graphql"} {
		if got := recebidos.distintos(onde); !mesmosValores(got, esperados) {
			t.Errorf("%s recebeu %v, esperava %v: algum valor declarado nunca chegou ao alvo", onde, got, esperados)
		}
	}
	if got := recebidos.distintos("corpo.regiao"); len(got) != 3 {
		t.Errorf("regiao recebeu %v, esperava tres valores distintos", got)
	}

	porNome := map[string]metrica.Variedade{}
	for _, variedade := range documento.Variedade {
		porNome[variedade.Nome] = variedade
	}
	if variedade := porNome["assinantes.id"]; variedade.Distintos != 3 {
		t.Errorf("o relatorio declarou %d valores distintos de assinantes.id, esperava 3", variedade.Distintos)
	}
	if variedade := porNome["assinantes.regiao"]; variedade.Distintos != 3 {
		t.Errorf("o relatorio declarou %d valores distintos de assinantes.regiao, esperava 3", variedade.Distintos)
	}
	for _, aviso := range documento.Avisos {
		if aviso.Tipo == "variedade_ausente" {
			t.Errorf("execucao com variedade nao pode gerar aviso de variedade ausente: %s", aviso.Evidencia)
		}
	}
}

// O aviso que teria pegado o bug do dado congelado. A fonte oferece tres
// valores; a execucao que usa um so e defeito, nao escolha.
func TestVariedadeDeUmValorSoViraAvisoGrave(t *testing.T) {
	documento := metrica.Documento{
		Variedade: []metrica.Variedade{
			{Nome: "assinantes.id", Distintos: 1, Usos: 2375, Disponivel: 3},
		},
	}
	avisos := metrica.AvisosDeVariedade(documento.Variedade)
	if len(avisos) != 1 {
		t.Fatalf("esperava um aviso, obtive %+v", avisos)
	}
	if avisos[0].Gravidade != metrica.GravidadeAlta {
		t.Errorf("gravidade = %q, esperava alta: o resultado nao representa a carga declarada", avisos[0].Gravidade)
	}
	if !strings.Contains(avisos[0].Mensagem, "cache") {
		t.Errorf("a mensagem precisa dizer por que isso engana: %q", avisos[0].Mensagem)
	}
	if !strings.Contains(avisos[0].Evidencia, "3 valores disponiveis") {
		t.Errorf("a evidencia precisa comparar o disponivel com o usado: %q", avisos[0].Evidencia)
	}
}

func TestValorFixoDeclaradoNoCenarioEhAvisoDeLeituraENaoDefeito(t *testing.T) {
	avisos := metrica.AvisosDeVariedade([]metrica.Variedade{
		{Nome: "pedidoFixo", Distintos: 1, Usos: 500, Disponivel: 0},
	})
	if len(avisos) != 1 || avisos[0].Gravidade != metrica.GravidadeMedia {
		t.Fatalf("valor fixo declarado e aviso de leitura, nao resultado invalido: %+v", avisos)
	}
}

func TestFonteComUmValorSoNaoGeraAviso(t *testing.T) {
	avisos := metrica.AvisosDeVariedade([]metrica.Variedade{
		{Nome: "assinantes.id", Distintos: 1, Usos: 500, Disponivel: 1},
	})
	if len(avisos) != 0 {
		t.Errorf("quem declarou um valor so nao precisa ser avisado disso: %+v", avisos)
	}
}

func executarCenarioDeVariedade(t *testing.T, endereco, csv string) metrica.Documento {
	t.Helper()
	raiz := t.TempDir()
	if err := os.WriteFile(filepath.Join(raiz, "assinantes.csv"), []byte(csv), 0o644); err != nil {
		t.Fatalf("nao consegui escrever o csv: %v", err)
	}
	caminho := filepath.Join(raiz, "cenario.yaml")
	if err := os.WriteFile(caminho, []byte(fmt.Sprintf(cenarioComVariedade, endereco)), 0o644); err != nil {
		t.Fatalf("nao consegui escrever o cenario: %v", err)
	}

	c, err := cenario.CarregarArquivo(caminho)
	if err != nil {
		t.Fatalf("cenario nao carregou: %v", err)
	}
	if err := c.Validar(); err != nil {
		t.Fatalf("cenario invalido: %v", err)
	}
	opcoes := motor.OpcoesPadrao()
	opcoes.RaizDeDados = raiz
	m, err := motor.Novo(c, opcoes)
	if err != nil {
		t.Fatalf("motor nao subiu: %v", err)
	}
	return m.Executar(context.Background())
}

func mesmosValores(obtidos, esperados []string) bool {
	if len(obtidos) != len(esperados) {
		return false
	}
	for indice, valor := range obtidos {
		if valor != esperados[indice] {
			return false
		}
	}
	return true
}
