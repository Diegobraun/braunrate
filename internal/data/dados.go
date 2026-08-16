package data

import (
	"encoding/csv"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Diegobraun/braunrate/internal/scenario"
)

type Fonte interface {
	Nome() string
	Proximo(usuarioVirtual int64) (map[string]string, error)
	Esgotada() bool
	// Quantos valores distintos cada variavel pode assumir. E o que permite ao
	// relatorio dizer que usar um valor so foi defeito, e nao um cenario que
	// declarou um valor so. Valor negativo significa indefinido.
	Disponiveis() map[string]int64
}

func Abrir(fonte scenario.FonteDeDados, raiz string) (Fonte, error) {
	if fonte.Sintetica() {
		return novaFonteSintetica(fonte)
	}
	return abrirCSV(fonte, raiz)
}

type fonteCSV struct {
	nome      string
	colunas   []string
	registros [][]string
	consumo   scenario.PoliticaDeConsumo
	posicao   atomic.Int64
	esgotada  atomic.Bool
	aleatorio *rand.Rand
	mutex     sync.Mutex
}

func abrirCSV(fonte scenario.FonteDeDados, raiz string) (Fonte, error) {
	caminho := fonte.Arquivo
	if !filepath.IsAbs(caminho) {
		caminho = filepath.Join(raiz, caminho)
	}
	arquivo, err := os.Open(caminho)
	if err != nil {
		return nil, fmt.Errorf("nao consegui abrir o arquivo de dados %q: %w", fonte.Arquivo, err)
	}
	defer arquivo.Close()

	leitor := csv.NewReader(arquivo)
	leitor.TrimLeadingSpace = true
	linhas, err := leitor.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("arquivo de dados %q invalido: %w", fonte.Arquivo, err)
	}
	if len(linhas) < 2 {
		return nil, fmt.Errorf("arquivo de dados %q precisa de cabecalho e pelo menos uma linha", fonte.Arquivo)
	}

	semente := fonte.Semente
	if semente == 0 {
		semente = 1
	}
	return &fonteCSV{
		nome:      fonte.Nome,
		colunas:   linhas[0],
		registros: linhas[1:],
		consumo:   fonte.Consumo,
		aleatorio: rand.New(rand.NewSource(semente)),
	}, nil
}

func (f *fonteCSV) Nome() string { return f.nome }

func (f *fonteCSV) Disponiveis() map[string]int64 {
	disponiveis := map[string]int64{}
	for posicao, coluna := range f.colunas {
		distintos := map[string]struct{}{}
		for _, registro := range f.registros {
			if posicao < len(registro) {
				distintos[registro[posicao]] = struct{}{}
			}
		}
		disponiveis[f.nome+"."+coluna] = int64(len(distintos))
	}
	return disponiveis
}

func (f *fonteCSV) Esgotada() bool { return f.esgotada.Load() }

func (f *fonteCSV) Proximo(usuarioVirtual int64) (map[string]string, error) {
	total := int64(len(f.registros))
	var indice int64

	switch f.consumo {
	case scenario.ConsumoAleatorio:
		f.mutex.Lock()
		indice = f.aleatorio.Int63n(total)
		f.mutex.Unlock()
	case scenario.ConsumoUnicoPorUsuario:
		indice = usuarioVirtual % total
	case scenario.ConsumoSequencial:
		indice = f.posicao.Add(1) - 1
		if indice >= total {
			f.esgotada.Store(true)
			return nil, fmt.Errorf("os dados de %q acabaram na linha %d; use consumo circular para repetir do inicio", f.nome, total)
		}
	default:
		indice = (f.posicao.Add(1) - 1) % total
	}

	registro := f.registros[indice]
	valores := make(map[string]string, len(f.colunas))
	for posicao, coluna := range f.colunas {
		if posicao < len(registro) {
			valores[f.nome+"."+coluna] = registro[posicao]
		}
	}
	return valores, nil
}

type fonteSintetica struct {
	nome           string
	campos         map[string]string
	nomesOrdenados []string
	semente        int64
	sequencia      atomic.Int64
}

func novaFonteSintetica(fonte scenario.FonteDeDados) (Fonte, error) {
	if len(fonte.Campos) == 0 {
		return nil, fmt.Errorf("a fonte de dados %q nao tem arquivo nem campos para gerar", fonte.Nome)
	}
	semente := fonte.Semente
	if semente == 0 {
		semente = 1
	}
	nomesOrdenados := make([]string, 0, len(fonte.Campos))
	for campo := range fonte.Campos {
		nomesOrdenados = append(nomesOrdenados, campo)
	}
	sort.Strings(nomesOrdenados)
	return &fonteSintetica{nome: fonte.Nome, campos: fonte.Campos,
		nomesOrdenados: nomesOrdenados, semente: semente}, nil
}

func (f *fonteSintetica) Nome() string { return f.nome }

func (f *fonteSintetica) Esgotada() bool { return false }

// Dado sintetico nao tem lista fechada de valores: o que se sabe e que gerar
// sempre o mesmo valor seria defeito.
func (f *fonteSintetica) Disponiveis() map[string]int64 {
	disponiveis := map[string]int64{}
	for _, campo := range f.nomesOrdenados {
		disponiveis[f.nome+"."+campo] = -1
	}
	return disponiveis
}

// A semente entra no relatorio de ambiente e os campos sao gerados em ordem
// fixa: sem as duas coisas a execucao nao e reproduzivel, e resultado nao
// reproduzivel nao serve para comparar duas execucoes.
func (f *fonteSintetica) Proximo(usuarioVirtual int64) (map[string]string, error) {
	sequencia := f.sequencia.Add(1)
	aleatorio := rand.New(rand.NewSource(f.semente + sequencia))
	valores := make(map[string]string, len(f.campos))
	for _, campo := range f.nomesOrdenados {
		valor, err := gerar(f.campos[campo], aleatorio, sequencia)
		if err != nil {
			return nil, fmt.Errorf("campo %q da fonte %q: %w", campo, f.nome, err)
		}
		valores[f.nome+"."+campo] = valor
	}
	return valores, nil
}

var nomes = []string{"ana", "bruno", "carla", "diego", "elisa", "fabio", "gabriela", "heitor", "isabel", "joao"}
var sobrenomes = []string{"souza", "lima", "braun", "costa", "martins", "azevedo", "ferreira", "rocha"}

func gerar(expressao string, aleatorio *rand.Rand, sequencia int64) (string, error) {
	nome, argumentos := separarGerador(expressao)
	switch nome {
	case "uuid":
		return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", aleatorio.Uint32(), aleatorio.Intn(0xffff),
			aleatorio.Intn(0xffff), aleatorio.Intn(0xffff), aleatorio.Int63n(0xffffffffffff)), nil
	case "sequencia":
		return strconv.FormatInt(sequencia, 10), nil
	case "numero":
		minimo, maximo := 0.0, 100.0
		if len(argumentos) == 2 {
			var err error
			if minimo, err = strconv.ParseFloat(argumentos[0], 64); err != nil {
				return "", fmt.Errorf("primeiro argumento de numero() invalido: %q", argumentos[0])
			}
			if maximo, err = strconv.ParseFloat(argumentos[1], 64); err != nil {
				return "", fmt.Errorf("segundo argumento de numero() invalido: %q", argumentos[1])
			}
		}
		if maximo <= minimo {
			return "", fmt.Errorf("numero(%v,%v) precisa de maximo maior que minimo", minimo, maximo)
		}
		return strconv.FormatFloat(minimo+aleatorio.Float64()*(maximo-minimo), 'f', 2, 64), nil
	case "inteiro":
		minimo, maximo := int64(0), int64(100)
		if len(argumentos) == 2 {
			minimo, _ = strconv.ParseInt(argumentos[0], 10, 64)
			maximo, _ = strconv.ParseInt(argumentos[1], 10, 64)
		}
		if maximo <= minimo {
			return "", fmt.Errorf("inteiro(%d,%d) precisa de maximo maior que minimo", minimo, maximo)
		}
		return strconv.FormatInt(minimo+aleatorio.Int63n(maximo-minimo), 10), nil
	case "nome":
		return nomes[aleatorio.Intn(len(nomes))] + " " + sobrenomes[aleatorio.Intn(len(sobrenomes))], nil
	case "email":
		return fmt.Sprintf("%s.%d@exemplo.com", nomes[aleatorio.Intn(len(nomes))], sequencia), nil
	case "texto":
		tamanho := 12
		if len(argumentos) == 1 {
			tamanho, _ = strconv.Atoi(argumentos[0])
		}
		letras := "abcdefghijklmnopqrstuvwxyz"
		construtor := strings.Builder{}
		for i := 0; i < tamanho; i++ {
			construtor.WriteByte(letras[aleatorio.Intn(len(letras))])
		}
		return construtor.String(), nil
	default:
		return "", fmt.Errorf("gerador desconhecido: %q (use uuid, sequencia, numero, inteiro, nome, email ou texto)", nome)
	}
}

func separarGerador(expressao string) (string, []string) {
	expressao = strings.TrimSpace(expressao)
	abertura := strings.Index(expressao, "(")
	if abertura < 0 || !strings.HasSuffix(expressao, ")") {
		return expressao, nil
	}
	nome := expressao[:abertura]
	conteudo := expressao[abertura+1 : len(expressao)-1]
	if strings.TrimSpace(conteudo) == "" {
		return nome, nil
	}
	argumentos := strings.Split(conteudo, ",")
	for indice := range argumentos {
		argumentos[indice] = strings.TrimSpace(argumentos[indice])
	}
	return nome, argumentos
}
