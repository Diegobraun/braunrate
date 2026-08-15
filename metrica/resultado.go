package metrica

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/Diegobraun/braunrate/protocolo"
)

const VersaoDoFormatoDeResultado = "1"

type Documento struct {
	VersaoDoFormato string             `json:"versao_do_formato"`
	Ferramenta      string             `json:"ferramenta"`
	Versao          string             `json:"versao"`
	Ambiente        Ambiente           `json:"ambiente"`
	Execucao        Execucao           `json:"execucao"`
	Agendamento     Agendamento        `json:"agendamento"`
	Passos          []ResultadoDePasso `json:"passos"`
	Global          ResultadoGlobal    `json:"global"`
	Avisos          []Aviso            `json:"avisos"`
	Series          []Bucket           `json:"series_temporais"`
}

type Ambiente struct {
	Maquina            string `json:"maquina"`
	SistemaOperacional string `json:"sistema_operacional"`
	Arquitetura        string `json:"arquitetura"`
	Nucleos            int    `json:"nucleos"`
	VersaoDoGo         string `json:"versao_do_go"`
}

type Execucao struct {
	Cenario           string           `json:"cenario"`
	Alvo              string           `json:"alvo"`
	Inicio            time.Time        `json:"inicio"`
	Fim               time.Time        `json:"fim"`
	DuracaoMs         int64            `json:"duracao_ms"`
	Modelo            string           `json:"modelo_de_chegada"`
	PlanoAplicado     []FaseAplicada   `json:"plano_aplicado"`
	MaximoSimultaneas int64            `json:"maximo_de_requisicoes_simultaneas"`
	Sementes          map[string]int64 `json:"sementes_dos_dados"`
	Autenticacoes     int64            `json:"obtencoes_de_autenticacao"`
}

type FaseAplicada struct {
	Tipo      string  `json:"tipo"`
	De        float64 `json:"de_por_segundo"`
	Ate       float64 `json:"ate_por_segundo"`
	DuracaoMs int64   `json:"duracao_ms"`
}

type Agendamento struct {
	Enviadas                  int64        `json:"enviadas"`
	Concluidas                int64        `json:"concluidas"`
	DespachosAtrasados        int64        `json:"despachos_atrasados"`
	DescartadasPorLimiteDeVoo int64        `json:"descartadas_por_limite_de_voo"`
	AmostrasPerdidas          int64        `json:"amostras_perdidas"`
	PicoEmVoo                 int64        `json:"pico_em_voo"`
	LimiarDeAtrasoMs          float64      `json:"limiar_de_atraso_ms"`
	Desvio                    Distribuicao `json:"desvio_de_agendamento"`
}

type ResultadoDePasso struct {
	Nome              string           `json:"nome"`
	Chave             string           `json:"chave"`
	Protocolo         string           `json:"protocolo"`
	Contagem          int64            `json:"contagem"`
	Sucessos          int64            `json:"sucessos"`
	Erros             int64            `json:"erros"`
	Bytes             int64            `json:"bytes"`
	ErrosPorClasse    map[string]int64 `json:"erros_por_classe"`
	StatusPorCodigo   map[string]int64 `json:"status_por_codigo"`
	Detalhes          map[string]int64 `json:"detalhes_de_erro"`
	Latencia          Distribuicao     `json:"latencia_corrigida"`
	LatenciaDeServico Distribuicao     `json:"latencia_de_servico"`
}

type ResultadoGlobal struct {
	Contagem          int64        `json:"contagem"`
	Sucessos          int64        `json:"sucessos"`
	Erros             int64        `json:"erros"`
	TaxaDeErro        float64      `json:"taxa_de_erro"`
	TaxaEfetiva       float64      `json:"taxa_efetiva_por_segundo"`
	Latencia          Distribuicao `json:"latencia_corrigida"`
	LatenciaDeServico Distribuicao `json:"latencia_de_servico"`
}

type Gravidade string

const (
	GravidadeAlta  Gravidade = "alta"
	GravidadeMedia Gravidade = "media"
	GravidadeBaixa Gravidade = "baixa"
)

type Aviso struct {
	Tipo      string    `json:"tipo"`
	Gravidade Gravidade `json:"gravidade"`
	Mensagem  string    `json:"mensagem"`
	Evidencia string    `json:"evidencia"`
}

func (d Documento) ResultadoValido() bool {
	for _, aviso := range d.Avisos {
		if aviso.Gravidade == GravidadeAlta {
			return false
		}
	}
	return true
}

type EntradaDoDocumento struct {
	Versao            string
	Cenario           string
	Alvo              string
	Modelo            string
	Inicio            time.Time
	Fim               time.Time
	Fases             []FaseAplicada
	MaximoSimultaneas int64
	Sementes          map[string]int64
	Autenticacoes     int64
}

func MontarDocumento(c *Coletor, entrada EntradaDoDocumento) Documento {
	maquina, _ := os.Hostname()
	documento := Documento{
		VersaoDoFormato: VersaoDoFormatoDeResultado,
		Ferramenta:      "braunrate",
		Versao:          entrada.Versao,
		Ambiente: Ambiente{
			Maquina:            maquina,
			SistemaOperacional: runtime.GOOS,
			Arquitetura:        runtime.GOARCH,
			Nucleos:            runtime.NumCPU(),
			VersaoDoGo:         runtime.Version(),
		},
		Execucao: Execucao{
			Cenario:           entrada.Cenario,
			Alvo:              entrada.Alvo,
			Inicio:            entrada.Inicio,
			Fim:               entrada.Fim,
			DuracaoMs:         entrada.Fim.Sub(entrada.Inicio).Milliseconds(),
			Modelo:            entrada.Modelo,
			PlanoAplicado:     entrada.Fases,
			MaximoSimultaneas: entrada.MaximoSimultaneas,
			Sementes:          entrada.Sementes,
			Autenticacoes:     entrada.Autenticacoes,
		},
		Agendamento: Agendamento{
			Enviadas:                  c.Enviadas,
			Concluidas:                c.Concluidas,
			DespachosAtrasados:        c.DespachosAtrasados,
			DescartadasPorLimiteDeVoo: c.DescartadasPorLimiteDeVoo,
			AmostrasPerdidas:          c.AmostrasPerdidas,
			PicoEmVoo:                 c.PicoEmVoo,
			LimiarDeAtrasoMs:          float64(c.LimiarDeAtraso.Microseconds()) / 1000,
			Desvio:                    c.DesvioDeAgendamento(),
		},
		Series: c.Buckets(),
	}

	agregados := c.Agregados()
	global := NovoAgregado("global", "global", "")
	for _, chave := range OrdenarChaves(agregados) {
		agregado := agregados[chave]
		global.Somar(agregado)
		documento.Passos = append(documento.Passos, converterPasso(agregado))
	}

	duracao := entrada.Fim.Sub(entrada.Inicio).Seconds()
	documento.Global = ResultadoGlobal{
		Contagem:          global.Contagem,
		Sucessos:          global.Sucessos,
		Erros:             global.Erros(),
		Latencia:          global.Distribuicao(),
		LatenciaDeServico: global.DistribuicaoDeServico(),
	}
	if global.Contagem > 0 {
		documento.Global.TaxaDeErro = float64(global.Erros()) / float64(global.Contagem)
	}
	if duracao > 0 {
		documento.Global.TaxaEfetiva = float64(global.Contagem) / duracao
	}

	documento.Avisos = avaliarAvisos(c, documento)
	return documento
}

func converterPasso(a *Agregado) ResultadoDePasso {
	errosPorClasse := map[string]int64{}
	for classe, quantidade := range a.ErrosPorClasse {
		errosPorClasse[string(classe)] = quantidade
	}
	statusPorCodigo := map[string]int64{}
	for status, quantidade := range a.StatusPorCodigo {
		statusPorCodigo[fmt.Sprintf("%d", status)] = quantidade
	}
	return ResultadoDePasso{
		Nome:              a.Passo,
		Chave:             a.Chave,
		Protocolo:         a.Protocolo,
		Contagem:          a.Contagem,
		Sucessos:          a.Sucessos,
		Erros:             a.Erros(),
		Bytes:             a.Bytes,
		ErrosPorClasse:    errosPorClasse,
		StatusPorCodigo:   statusPorCodigo,
		Detalhes:          a.Detalhes,
		Latencia:          a.Distribuicao(),
		LatenciaDeServico: a.DistribuicaoDeServico(),
	}
}

func avaliarAvisos(c *Coletor, documento Documento) []Aviso {
	var avisos []Aviso
	agendamento := documento.Agendamento

	if agendamento.DescartadasPorLimiteDeVoo > 0 {
		avisos = append(avisos, Aviso{
			Tipo:      "gerador_saturado",
			Gravidade: GravidadeAlta,
			Mensagem:  "o gerador atingiu o limite de requisicoes em voo e deixou de enviar requisicoes agendadas; o resultado nao vale",
			Evidencia: fmt.Sprintf("%d requisicoes descartadas, pico de %d em voo", agendamento.DescartadasPorLimiteDeVoo, agendamento.PicoEmVoo),
		})
	}

	if agendamento.Enviadas > 0 {
		proporcao := float64(agendamento.DespachosAtrasados) / float64(agendamento.Enviadas)
		if proporcao >= 0.01 {
			avisos = append(avisos, Aviso{
				Tipo:      "gerador_saturado",
				Gravidade: GravidadeAlta,
				Mensagem:  "o gerador nao sustentou a taxa alvo: despachos sairam depois do instante agendado; o resultado nao vale",
				Evidencia: fmt.Sprintf("%.2f%% dos despachos atrasaram mais de %.1f ms (desvio p99 de %.1f ms)", proporcao*100, agendamento.LimiarDeAtrasoMs, agendamento.Desvio.P99),
			})
		} else if agendamento.DespachosAtrasados > 0 {
			avisos = append(avisos, Aviso{
				Tipo:      "gerador_com_atraso_pontual",
				Gravidade: GravidadeBaixa,
				Mensagem:  "houve atraso pontual de despacho, abaixo de 1% das requisicoes",
				Evidencia: fmt.Sprintf("%d despachos atrasados de %d (desvio p99 de %.1f ms)", agendamento.DespachosAtrasados, agendamento.Enviadas, agendamento.Desvio.P99),
			})
		}
	}

	if agendamento.AmostrasPerdidas > 0 {
		avisos = append(avisos, Aviso{
			Tipo:      "amostras_perdidas",
			Gravidade: GravidadeAlta,
			Mensagem:  "o coletor de metrica nao acompanhou o volume e perdeu amostras; a distribuicao esta incompleta",
			Evidencia: fmt.Sprintf("%d amostras perdidas", agendamento.AmostrasPerdidas),
		})
	}

	if aviso, houve := detectarDegradacaoDoAlvo(documento); houve {
		avisos = append(avisos, aviso)
	}

	return avisos
}

func detectarDegradacaoDoAlvo(documento Documento) (Aviso, bool) {
	series := documento.Series
	if len(series) < 4 {
		return Aviso{}, false
	}
	primeiro := series[0].LatenciaP99Ms
	if primeiro <= 0 {
		primeiro = series[1].LatenciaP99Ms
	}
	pior := 0.0
	for _, bucket := range series {
		if bucket.LatenciaP99Ms > pior {
			pior = bucket.LatenciaP99Ms
		}
	}
	if primeiro > 0 && pior >= 3*primeiro {
		return Aviso{
			Tipo:      "alvo_degradado",
			Gravidade: GravidadeMedia,
			Mensagem:  "a latencia do alvo cresceu ao longo da execucao enquanto o despacho continuou pontual; a degradacao e do alvo, nao do gerador",
			Evidencia: fmt.Sprintf("p99 por segundo passou de %.1f ms para %.1f ms", primeiro, pior),
		}, true
	}
	return Aviso{}, false
}

func ClasseLegivel(classe protocolo.ClasseDeErro) string {
	switch classe {
	case protocolo.ErroDeRede:
		return "falha de rede"
	case protocolo.ErroDeTimeout:
		return "timeout"
	case protocolo.ErroDeStatus:
		return "status HTTP inesperado"
	case protocolo.ErroDeAssercao:
		return "assercao funcional"
	case protocolo.ErroDeCorrelacao:
		return "correlacao perdida"
	case protocolo.ErroDeSaturacao:
		return "saturacao do gerador"
	default:
		return string(classe)
	}
}
