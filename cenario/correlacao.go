package cenario

import (
	"time"
)

type OrigemDaCaptura string

const (
	CapturaDeJSON      OrigemDaCaptura = "json"
	CapturaDeCabecalho OrigemDaCaptura = "cabecalho"
	CapturaDeRegex     OrigemDaCaptura = "regex"
	CapturaDeCorpo     OrigemDaCaptura = "corpo"
	CapturaDeStatus    OrigemDaCaptura = "status"
)

type Captura struct {
	Variavel    string
	Origem      OrigemDaCaptura
	Expressao   string
	Obrigatoria bool
	Padrao      string
	Linha       int
}

type TipoDeAssercao string

const (
	AsserirStatus      TipoDeAssercao = "status"
	AsserirCorpoContem TipoDeAssercao = "corpo_contem"
	AsserirJSON        TipoDeAssercao = "json"
	AsserirRegex       TipoDeAssercao = "regex"
	AsserirCabecalho   TipoDeAssercao = "cabecalho"
)

type Assercao struct {
	Tipo      TipoDeAssercao
	Alvo      string
	Operador  Operador
	Valor     string
	Descricao string
	Linha     int
}

type Operador string

const (
	OperadorIgual        Operador = "=="
	OperadorDiferente    Operador = "!="
	OperadorMenor        Operador = "<"
	OperadorMenorOuIgual Operador = "<="
	OperadorMaior        Operador = ">"
	OperadorMaiorOuIgual Operador = ">="
	OperadorContem       Operador = "contem"
	OperadorExiste       Operador = "existe"
)

type TipoDeAutenticacao string

const (
	AutenticacaoPorToken  TipoDeAutenticacao = "token"
	AutenticacaoBasica    TipoDeAutenticacao = "basica"
	AutenticacaoCabecalho TipoDeAutenticacao = "cabecalho"
)

type Autenticacao struct {
	Tipo        TipoDeAutenticacao
	Obter       *Passo
	RenovarApos time.Duration
	Cabecalho   string
	Usuario     string
	Senha       string
	Linha       int
}

type PoliticaDeConsumo string

const (
	ConsumoSequencial      PoliticaDeConsumo = "sequencial"
	ConsumoAleatorio       PoliticaDeConsumo = "aleatorio"
	ConsumoCircular        PoliticaDeConsumo = "circular"
	ConsumoUnicoPorUsuario PoliticaDeConsumo = "unico_por_usuario"
)

type FonteDeDados struct {
	Nome      string
	Arquivo   string
	Consumo   PoliticaDeConsumo
	Semente   int64
	Campos    map[string]string
	Registros int
	Linha     int
}

func (f FonteDeDados) Sintetica() bool {
	return f.Arquivo == ""
}

type RegraDeSLO struct {
	Passo    string
	Global   bool
	Metrica  string
	Operador Operador
	Limite   float64
	Unidade  string
	Texto    string
	Linha    int
}
