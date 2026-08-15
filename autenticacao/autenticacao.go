package autenticacao

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Diegobraun/braunrate/cenario"
	"github.com/Diegobraun/braunrate/contexto"
	"github.com/Diegobraun/braunrate/protocolo"
)

type Relogio interface {
	Agora() time.Time
}

type ExecutarPasso func(ctx context.Context, passo cenario.Passo, valores *contexto.Contexto) (protocolo.Resposta, error)

type Gerenciador struct {
	configuracao cenario.Autenticacao
	executar     ExecutarPasso
	relogio      Relogio

	mutex     sync.Mutex
	valores   map[string]string
	obtido    bool
	obtidoEm  time.Time
	Obtencoes int64
}

func Novo(configuracao cenario.Autenticacao, executar ExecutarPasso, relogio Relogio) *Gerenciador {
	return &Gerenciador{configuracao: configuracao, executar: executar, relogio: relogio, valores: map[string]string{}}
}

// O token e obtido uma vez e renovado quando vence, nunca por requisicao: o
// autor declara a renovacao e o motor cuida de quando ela acontece.
func (g *Gerenciador) Cabecalho(ctx context.Context, valores *contexto.Contexto) (string, string, error) {
	if g.configuracao.Tipo == cenario.AutenticacaoBasica {
		usuario := valores.Resolver(g.configuracao.Usuario)
		senha := valores.Resolver(g.configuracao.Senha)
		credencial := base64.StdEncoding.EncodeToString([]byte(usuario + ":" + senha))
		return "Authorization", "Basic " + credencial, nil
	}

	if err := g.garantirToken(ctx, valores); err != nil {
		return "", "", err
	}

	g.mutex.Lock()
	obtidos := make(map[string]string, len(g.valores))
	for nome, valor := range g.valores {
		obtidos[nome] = valor
	}
	g.mutex.Unlock()

	valores.DefinirVarios(obtidos)

	nome, modelo, encontrou := strings.Cut(g.configuracao.Cabecalho, ":")
	if !encontrou {
		return "", "", fmt.Errorf("o cabecalho de autenticacao precisa ser \"Nome: valor\", recebido %q", g.configuracao.Cabecalho)
	}
	return strings.TrimSpace(nome), strings.TrimSpace(valores.Resolver(modelo)), nil
}

func (g *Gerenciador) garantirToken(ctx context.Context, valores *contexto.Contexto) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	if g.obtido && !g.venceu() {
		return nil
	}

	entrada := valores.Valores()
	valoresDaObtencao := contexto.Novo(0, 0, entrada)
	resposta, err := g.executar(ctx, *g.configuracao.Obter, valoresDaObtencao)
	if err != nil {
		return fmt.Errorf("nao consegui obter a autenticacao: %w", err)
	}
	if resposta.Status >= 400 {
		return fmt.Errorf("a requisicao de autenticacao respondeu %d; confira usuario, senha e caminho em 'autenticacao.obter'", resposta.Status)
	}

	// So o que a requisicao de autenticacao produziu fica guardado. Guardar o
	// contexto inteiro congelaria os dados da primeira iteracao e os reinjetaria
	// em todas as outras: toda a carga cairia sobre a primeira linha do CSV, com
	// o relatorio afirmando variedade que nao existiu.
	obtidos := map[string]string{}
	for nome, valor := range valoresDaObtencao.Valores() {
		if anterior, existia := entrada[nome]; existia && anterior == valor {
			continue
		}
		obtidos[nome] = valor
	}
	g.valores = obtidos
	g.obtido = true
	g.obtidoEm = g.relogio.Agora()
	g.Obtencoes++
	return nil
}

func (g *Gerenciador) venceu() bool {
	if g.configuracao.RenovarApos <= 0 {
		return false
	}
	return g.relogio.Agora().Sub(g.obtidoEm) >= g.configuracao.RenovarApos
}
