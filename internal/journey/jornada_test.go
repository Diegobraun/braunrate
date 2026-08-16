package journey

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/engine"
	"github.com/Diegobraun/braunrate/internal/metrics"
	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	"github.com/Diegobraun/braunrate/internal/scenario"
	"github.com/Diegobraun/braunrate/internal/slo"
	"github.com/Diegobraun/braunrate/internal/testsupport"
)

const modeloDoCenario = `
nome: Jornada de cobranca
alvo: %s

variaveis:
  usuario: ${USUARIO_DE_TESTE:-ana}

autenticacao:
  tipo: token
  obter:
    http:
      metodo: POST
      caminho: /auth/token
      corpo: { usuario: "${usuario}", senha: "${SENHA:-segredo}" }
    captura: { token: $.access_token }
  renovar_apos: 25m

dados:
  assinantes:
    arquivo: assinantes.csv
    consumo: circular

carga:
  perfis:
    - constante: { taxa: 100/s, durante: 2s }

cenario:
  - http: GET /pedidos/${assinantes.id}
    nome: consultar pedido
    verificar:
      status: 200
      json: { $.ultimaFatura.status: ABERTA }
    captura:
      faturaId: $.ultimaFatura.id

  - nome: pagar fatura
    http:
      metodo: POST
      caminho: /faturas/${faturaId}/pagar
      corpo: { valor: 199.90 }
    verificar:
      status: 200
      json: { $.status: PAGA }

slo:
  - consultar pedido: { p95: < 2s }
  - global: { erros: < 0.1 }
`

func prepararCenario(t *testing.T, endereco string) (scenario.Cenario, string) {
	t.Helper()
	raiz := t.TempDir()
	if err := os.WriteFile(filepath.Join(raiz, "assinantes.csv"),
		[]byte("id,nome\n1001,ana\n1002,bruno\n1003,carla\n"), 0o644); err != nil {
		t.Fatalf("nao consegui escrever o csv: %v", err)
	}

	conteudo := fmt.Sprintf(modeloDoCenario, endereco)
	caminho := filepath.Join(raiz, "jornada.yaml")
	if err := os.WriteFile(caminho, []byte(conteudo), 0o644); err != nil {
		t.Fatalf("nao consegui escrever o cenario: %v", err)
	}

	c, err := scenario.CarregarArquivo(caminho)
	if err != nil {
		t.Fatalf("cenario nao carregou: %v", err)
	}
	if err := c.Validar(); err != nil {
		t.Fatalf("cenario invalido: %v", err)
	}
	return c, raiz
}

func executar(t *testing.T) (metrics.Documento, slo.Veredito) {
	t.Helper()
	servidor := testsupport.Novo(testsupport.Opcoes{Latencia: time.Millisecond})
	if err := servidor.Iniciar("127.0.0.1:0"); err != nil {
		t.Fatalf("alvo nao subiu: %v", err)
	}
	t.Cleanup(func() { _ = servidor.Encerrar() })

	c, raiz := prepararCenario(t, servidor.Endereco())
	opcoes := engine.OpcoesPadrao()
	opcoes.RaizDeDados = raiz
	opcoes.Versao = "teste"

	m, err := engine.Novo(c, opcoes)
	if err != nil {
		t.Fatalf("motor nao subiu: %v", err)
	}
	documento := m.Executar(context.Background())
	return documento, slo.Avaliar(c.SLO, documento)
}

func TestJornadaComAutenticacaoCorrelacaoEDadosFuncionaPontaAPonta(t *testing.T) {
	documento, veredito := executar(t)

	if documento.Global.Erros != 0 {
		t.Fatalf("esperava zero erro, obtive %d: %+v", documento.Global.Erros, documento.Passos)
	}
	if len(documento.Passos) != 2 {
		t.Fatalf("esperava dois passos no relatorio, obtive %d", len(documento.Passos))
	}
	for _, passo := range documento.Passos {
		if passo.Contagem != 200 {
			t.Errorf("passo %q com %d requisicoes, esperado 200", passo.Nome, passo.Contagem)
		}
	}
	if !veredito.Passou {
		t.Errorf("SLO deveria passar: %s", veredito.Frase)
	}
	if documento.Execucao.Autenticacoes != 1 {
		t.Errorf("autenticacoes = %d, esperado 1 para a execucao inteira", documento.Execucao.Autenticacoes)
	}
}

func TestCorrelacaoQuebradaViraErroDeCorrelacaoENaoErroDeRede(t *testing.T) {
	servidor := testsupport.Novo(testsupport.Opcoes{Latencia: time.Millisecond})
	if err := servidor.Iniciar("127.0.0.1:0"); err != nil {
		t.Fatalf("alvo nao subiu: %v", err)
	}
	t.Cleanup(func() { _ = servidor.Encerrar() })

	c, raiz := prepararCenario(t, servidor.Endereco())
	c.Passos[0].Capturas[0].Expressao = "$.campo.que.nao.existe"

	opcoes := engine.OpcoesPadrao()
	opcoes.RaizDeDados = raiz
	m, err := engine.Novo(c, opcoes)
	if err != nil {
		t.Fatalf("motor nao subiu: %v", err)
	}
	documento := m.Executar(context.Background())

	primeiro := documento.Passos[0]
	if primeiro.ErrosPorClasse["correlacao"] == 0 {
		t.Fatalf("esperava erro de correlacao, obtive %+v", primeiro.ErrosPorClasse)
	}
	if len(documento.Passos) != 1 {
		t.Errorf("o passo seguinte nao deveria ter rodado sem a captura; passos: %d", len(documento.Passos))
	}
	achouExplicacao := false
	for detalhe := range primeiro.Detalhes {
		if strings.Contains(detalhe, "campo.que.nao.existe") {
			achouExplicacao = true
		}
	}
	if !achouExplicacao {
		t.Errorf("o relatorio precisa dizer qual captura falhou: %+v", primeiro.Detalhes)
	}
}

func TestAssercaoQuebradaSeparaFalhaFuncionalDeFalhaDeSLO(t *testing.T) {
	servidor := testsupport.Novo(testsupport.Opcoes{Latencia: time.Millisecond})
	if err := servidor.Iniciar("127.0.0.1:0"); err != nil {
		t.Fatalf("alvo nao subiu: %v", err)
	}
	t.Cleanup(func() { _ = servidor.Encerrar() })

	c, raiz := prepararCenario(t, servidor.Endereco())
	c.Passos[0].Assercoes = []scenario.Assercao{{
		Tipo: scenario.AsserirJSON, Alvo: "$.ultimaFatura.status",
		Operador: scenario.OperadorIgual, Valor: "PAGA",
	}}

	opcoes := engine.OpcoesPadrao()
	opcoes.RaizDeDados = raiz
	m, _ := engine.Novo(c, opcoes)
	documento := m.Executar(context.Background())
	veredito := slo.Avaliar(c.SLO, documento)

	if documento.Passos[0].ErrosPorClasse["assercao"] == 0 {
		t.Fatalf("esperava falha funcional classificada como assercao: %+v", documento.Passos[0].ErrosPorClasse)
	}
	if veredito.Passou {
		t.Error("o SLO global de erros deveria falhar junto")
	}
	if !strings.Contains(veredito.Frase, "taxa de erro") {
		t.Errorf("a frase do veredito deveria falar de taxa de erro: %q", veredito.Frase)
	}
}
