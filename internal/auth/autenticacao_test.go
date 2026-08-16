package auth_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/internal/auth"
	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/Diegobraun/braunrate/internal/runtime"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

type relogioDeTeste struct{ agora time.Time }

func (r *relogioDeTeste) Agora() time.Time { return r.agora }

func (r *relogioDeTeste) Avancar(duracao time.Duration) { r.agora = r.agora.Add(duracao) }

func configuracaoDeToken(renovarApos time.Duration) scenario.Autenticacao {
	return scenario.Autenticacao{
		Tipo:        scenario.AutenticacaoPorToken,
		Obter:       &scenario.Passo{Nome: "obter autenticacao", Protocolo: "http"},
		RenovarApos: renovarApos,
		Cabecalho:   "Authorization: Bearer ${token}",
	}
}

func TestTokenEhObtidoUmaVezEReaproveitado(t *testing.T) {
	relogio := &relogioDeTeste{agora: time.Unix(1_700_000_000, 0)}
	obtencoes := 0
	executar := func(_ context.Context, _ scenario.Passo, valores *runtime.Contexto) (protocol.Resposta, error) {
		obtencoes++
		valores.Definir("token", "token-de-teste")
		return protocol.Resposta{Status: 200}, nil
	}

	gerenciador := auth.Novo(configuracaoDeToken(25*time.Minute), executar, relogio)

	for i := 0; i < 50; i++ {
		nome, valor, err := gerenciador.Cabecalho(context.Background(), runtime.Novo(int64(i), 0, nil))
		if err != nil {
			t.Fatalf("iteracao %d: %v", i, err)
		}
		if nome != "Authorization" || valor != "Bearer token-de-teste" {
			t.Fatalf("cabecalho = %q: %q", nome, valor)
		}
	}
	if obtencoes != 1 {
		t.Errorf("obtencoes = %d, esperado 1: o token e obtido uma vez, nao por requisicao", obtencoes)
	}
}

func TestTokenEhRenovadoQuandoVence(t *testing.T) {
	relogio := &relogioDeTeste{agora: time.Unix(1_700_000_000, 0)}
	obtencoes := 0
	executar := func(_ context.Context, _ scenario.Passo, valores *runtime.Contexto) (protocol.Resposta, error) {
		obtencoes++
		valores.Definir("token", "token-"+time.Duration(obtencoes).String())
		return protocol.Resposta{Status: 200}, nil
	}

	gerenciador := auth.Novo(configuracaoDeToken(25*time.Minute), executar, relogio)

	if _, _, err := gerenciador.Cabecalho(context.Background(), runtime.Novo(0, 0, nil)); err != nil {
		t.Fatalf("primeira obtencao: %v", err)
	}
	relogio.Avancar(24 * time.Minute)
	if _, _, err := gerenciador.Cabecalho(context.Background(), runtime.Novo(1, 0, nil)); err != nil {
		t.Fatalf("antes de vencer: %v", err)
	}
	if obtencoes != 1 {
		t.Fatalf("obtencoes = %d antes de vencer, esperado 1", obtencoes)
	}

	relogio.Avancar(2 * time.Minute)
	if _, _, err := gerenciador.Cabecalho(context.Background(), runtime.Novo(2, 0, nil)); err != nil {
		t.Fatalf("apos vencer: %v", err)
	}
	if obtencoes != 2 {
		t.Errorf("obtencoes = %d apos vencer, esperado 2", obtencoes)
	}
}

func TestFalhaDeAutenticacaoExplicaOQueConferir(t *testing.T) {
	relogio := &relogioDeTeste{agora: time.Unix(1_700_000_000, 0)}
	executar := func(context.Context, scenario.Passo, *runtime.Contexto) (protocol.Resposta, error) {
		return protocol.Resposta{Status: 401}, nil
	}

	gerenciador := auth.Novo(configuracaoDeToken(0), executar, relogio)
	_, _, err := gerenciador.Cabecalho(context.Background(), runtime.Novo(0, 0, nil))
	if err == nil {
		t.Fatal("esperava erro")
	}
	for _, trecho := range []string{"401", "usuario", "senha", "autenticacao.obter"} {
		if !strings.Contains(err.Error(), trecho) {
			t.Errorf("mensagem %q nao menciona %q", err.Error(), trecho)
		}
	}
}

func TestAutenticacaoBasicaNaoFazRequisicao(t *testing.T) {
	relogio := &relogioDeTeste{agora: time.Unix(1_700_000_000, 0)}
	executar := func(context.Context, scenario.Passo, *runtime.Contexto) (protocol.Resposta, error) {
		t.Fatal("autenticacao basica nao deveria fazer requisicao")
		return protocol.Resposta{}, nil
	}

	configuracao := scenario.Autenticacao{Tipo: scenario.AutenticacaoBasica, Usuario: "ana", Senha: "${SENHA}"}
	gerenciador := auth.Novo(configuracao, executar, relogio)

	valores := runtime.Novo(0, 0, map[string]string{"SENHA": "segredo"})
	nome, valor, err := gerenciador.Cabecalho(context.Background(), valores)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if nome != "Authorization" || valor != "Basic YW5hOnNlZ3JlZG8=" {
		t.Errorf("cabecalho = %q: %q", nome, valor)
	}
}
