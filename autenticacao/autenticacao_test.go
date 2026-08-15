package autenticacao_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Diegobraun/braunrate/autenticacao"
	"github.com/Diegobraun/braunrate/cenario"
	"github.com/Diegobraun/braunrate/contexto"
	"github.com/Diegobraun/braunrate/protocolo"
)

type relogioDeTeste struct{ agora time.Time }

func (r *relogioDeTeste) Agora() time.Time { return r.agora }

func (r *relogioDeTeste) Avancar(duracao time.Duration) { r.agora = r.agora.Add(duracao) }

func configuracaoDeToken(renovarApos time.Duration) cenario.Autenticacao {
	return cenario.Autenticacao{
		Tipo:        cenario.AutenticacaoPorToken,
		Obter:       &cenario.Passo{Nome: "obter autenticacao", Protocolo: "http"},
		RenovarApos: renovarApos,
		Cabecalho:   "Authorization: Bearer ${token}",
	}
}

func TestTokenEhObtidoUmaVezEReaproveitado(t *testing.T) {
	relogio := &relogioDeTeste{agora: time.Unix(1_700_000_000, 0)}
	obtencoes := 0
	executar := func(_ context.Context, _ cenario.Passo, valores *contexto.Contexto) (protocolo.Resposta, error) {
		obtencoes++
		valores.Definir("token", "token-de-teste")
		return protocolo.Resposta{Status: 200}, nil
	}

	gerenciador := autenticacao.Novo(configuracaoDeToken(25*time.Minute), executar, relogio)

	for i := 0; i < 50; i++ {
		nome, valor, err := gerenciador.Cabecalho(context.Background(), contexto.Novo(int64(i), 0, nil))
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
	executar := func(_ context.Context, _ cenario.Passo, valores *contexto.Contexto) (protocolo.Resposta, error) {
		obtencoes++
		valores.Definir("token", "token-"+time.Duration(obtencoes).String())
		return protocolo.Resposta{Status: 200}, nil
	}

	gerenciador := autenticacao.Novo(configuracaoDeToken(25*time.Minute), executar, relogio)

	if _, _, err := gerenciador.Cabecalho(context.Background(), contexto.Novo(0, 0, nil)); err != nil {
		t.Fatalf("primeira obtencao: %v", err)
	}
	relogio.Avancar(24 * time.Minute)
	if _, _, err := gerenciador.Cabecalho(context.Background(), contexto.Novo(1, 0, nil)); err != nil {
		t.Fatalf("antes de vencer: %v", err)
	}
	if obtencoes != 1 {
		t.Fatalf("obtencoes = %d antes de vencer, esperado 1", obtencoes)
	}

	relogio.Avancar(2 * time.Minute)
	if _, _, err := gerenciador.Cabecalho(context.Background(), contexto.Novo(2, 0, nil)); err != nil {
		t.Fatalf("apos vencer: %v", err)
	}
	if obtencoes != 2 {
		t.Errorf("obtencoes = %d apos vencer, esperado 2", obtencoes)
	}
}

func TestFalhaDeAutenticacaoExplicaOQueConferir(t *testing.T) {
	relogio := &relogioDeTeste{agora: time.Unix(1_700_000_000, 0)}
	executar := func(context.Context, cenario.Passo, *contexto.Contexto) (protocolo.Resposta, error) {
		return protocolo.Resposta{Status: 401}, nil
	}

	gerenciador := autenticacao.Novo(configuracaoDeToken(0), executar, relogio)
	_, _, err := gerenciador.Cabecalho(context.Background(), contexto.Novo(0, 0, nil))
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
	executar := func(context.Context, cenario.Passo, *contexto.Contexto) (protocolo.Resposta, error) {
		t.Fatal("autenticacao basica nao deveria fazer requisicao")
		return protocolo.Resposta{}, nil
	}

	configuracao := cenario.Autenticacao{Tipo: cenario.AutenticacaoBasica, Usuario: "ana", Senha: "${SENHA}"}
	gerenciador := autenticacao.Novo(configuracao, executar, relogio)

	valores := contexto.Novo(0, 0, map[string]string{"SENHA": "segredo"})
	nome, valor, err := gerenciador.Cabecalho(context.Background(), valores)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if nome != "Authorization" || valor != "Basic YW5hOnNlZ3JlZG8=" {
		t.Errorf("cabecalho = %q: %q", nome, valor)
	}
}
