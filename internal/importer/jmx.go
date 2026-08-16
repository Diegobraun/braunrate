package importer

import (
	"encoding/xml"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type elemento struct {
	XMLName   xml.Name
	Atributos []xml.Attr  `xml:",any,attr"`
	Filhos    []*elemento `xml:",any"`
	Texto     string      `xml:",chardata"`
}

func (e *elemento) atributo(nome string) string {
	for _, atributo := range e.Atributos {
		if atributo.Name.Local == nome {
			return atributo.Value
		}
	}
	return ""
}

// Propriedade de JMeter e sempre "<stringProp name=...>valor</stringProp>",
// inclusive dentro de elementProp aninhado, entao a busca desce a arvore.
func (e *elemento) propriedade(nome string) string {
	for _, filho := range e.Filhos {
		if filho.atributo("name") == nome && len(filho.Filhos) == 0 {
			return strings.TrimSpace(filho.Texto)
		}
		if valor := filho.propriedade(nome); valor != "" {
			return valor
		}
	}
	return ""
}

func (e *elemento) buscarTodos(nome string) []*elemento {
	var achados []*elemento
	if e.XMLName.Local == nome {
		achados = append(achados, e)
	}
	for _, filho := range e.Filhos {
		achados = append(achados, filho.buscarTodos(nome)...)
	}
	return achados
}

// A traducao e parcial de proposito e o que ficou de fora sai declarado: um
// importador que engole o arquivo inteiro em silencio entrega um cenario que
// mede outra coisa e ninguem percebe.
var elementosTraduzidos = map[string]bool{
	"jmeterTestPlan": true, "hashTree": true, "TestPlan": true, "ThreadGroup": true,
	"HTTPSamplerProxy": true, "HeaderManager": true, "CSVDataSet": true,
	"JSONPostProcessor": true, "RegexExtractor": true, "ResponseAssertion": true,
	"elementProp": true, "collectionProp": true, "stringProp": true, "boolProp": true,
	"intProp": true, "longProp": true, "doubleProp": true, "objProp": true,
}

func DeJMX(conteudo []byte) (Importacao, error) {
	var raiz elemento
	if err := xml.Unmarshal(conteudo, &raiz); err != nil {
		return Importacao{}, fmt.Errorf("nao consegui ler o arquivo como .jmx: %v", err)
	}

	amostradores := raiz.buscarTodos("HTTPSamplerProxy")
	if len(amostradores) == 0 {
		return Importacao{}, fmt.Errorf(`nao achei nenhuma requisicao HTTP no .jmx.
Hoje o importador traduz HTTPSamplerProxy (requisicao HTTP), HeaderManager, CSVDataSet,
extratores JSON e regex, e assercao de resposta. Sampler de JDBC, JMS ou script nao entra`)
	}

	roteiro := Roteiro{Nome: nomeDoPlano(&raiz), Passos: nil}
	cabecalhosGlobais := map[string]string{}
	for _, gerente := range raiz.buscarTodos("HeaderManager") {
		for nome, valor := range cabecalhosDe(gerente) {
			cabecalhosGlobais[nome] = valor
		}
	}

	alvos := map[string]int{}
	usados := map[string]int{}
	for _, amostrador := range amostradores {
		passo, alvo := passoDeAmostrador(amostrador, cabecalhosGlobais)
		if alvo != "" {
			alvos[alvo]++
		}
		nome := passo.Nome
		usados[nome]++
		if usados[nome] > 1 {
			passo.Nome = fmt.Sprintf("%s %d", nome, usados[nome])
		}
		roteiro.Passos = append(roteiro.Passos, passo)
	}

	roteiro.Alvo = alvoMaisComum(alvos)
	if roteiro.Alvo == "" {
		roteiro.Alvo = "http://127.0.0.1:8080"
		roteiro.Avisos = append(roteiro.Avisos,
			"o .jmx nao declara dominio nas requisicoes (provavelmente usa variavel de plano): troque o alvo antes de rodar")
	}
	if len(alvos) > 1 {
		roteiro.Avisos = append(roteiro.Avisos,
			fmt.Sprintf("o .jmx aponta para %d dominios diferentes; ficou o mais frequente e os outros viraram caminho fixo", len(alvos)))
	}

	for _, conjunto := range raiz.buscarTodos("CSVDataSet") {
		roteiro.Dados = append(roteiro.Dados, fonteDeCSV(conjunto))
	}

	for _, passo := range roteiro.Passos {
		if temIdentificador(passo.Caminho) {
			roteiro.Avisos = append(roteiro.Avisos, fmt.Sprintf(
				"o passo %q tem valor fixo no caminho (%s): com um valor so, o alvo responde de cache e o numero fica otimista. "+
					"Troque por ${dados.coluna} e aponte para o CSV", passo.Nome, passo.Caminho))
		}
	}
	roteiro.Avisos = append(roteiro.Avisos, avisosDeCarga(&raiz)...)
	roteiro.Avisos = append(roteiro.Avisos, avisosDeCorrelacao(&raiz)...)
	roteiro.Avisos = append(roteiro.Avisos, avisosDoQueFicouDeFora(&raiz)...)

	importacao := GerarYAML(roteiro)
	return importacao, nil
}

func nomeDoPlano(raiz *elemento) string {
	for _, plano := range raiz.buscarTodos("TestPlan") {
		if nome := strings.TrimSpace(plano.atributo("testname")); nome != "" {
			return "Importado de JMeter: " + nome
		}
	}
	return "Importado de JMeter"
}

func passoDeAmostrador(amostrador *elemento, globais map[string]string) (PassoImportado, string) {
	metodo := strings.ToUpper(amostrador.propriedade("HTTPSampler.method"))
	if metodo == "" {
		metodo = "GET"
	}
	caminho := amostrador.propriedade("HTTPSampler.path")
	if caminho == "" {
		caminho = "/"
	}
	if !strings.HasPrefix(caminho, "/") && !strings.HasPrefix(caminho, "http") {
		caminho = "/" + caminho
	}

	passo := PassoImportado{
		Metodo:     metodo,
		Caminho:    caminho,
		Cabecalhos: map[string]string{},
		Corpo:      corpoDeAmostrador(amostrador),
	}
	for nome, valor := range globais {
		passo.Cabecalhos[nome] = valor
	}

	nome := strings.TrimSpace(amostrador.atributo("testname"))
	if nome == "" || nome == "HTTP Request" {
		nome = strings.ToLower(metodo) + " " + recurso(caminho)
	}
	passo.Nome = nome

	dominio := amostrador.propriedade("HTTPSampler.domain")
	if dominio == "" {
		return passo, ""
	}
	esquema := amostrador.propriedade("HTTPSampler.protocol")
	if esquema == "" {
		esquema = "http"
	}
	alvo := esquema + "://" + dominio
	if porta := amostrador.propriedade("HTTPSampler.port"); porta != "" && porta != "80" && porta != "443" {
		alvo += ":" + porta
	}
	return passo, alvo
}

func corpoDeAmostrador(amostrador *elemento) string {
	for _, argumento := range amostrador.buscarTodos("elementProp") {
		if argumento.atributo("elementType") != "HTTPArgument" {
			continue
		}
		if valor := argumento.propriedade("Argument.value"); valor != "" {
			return valor
		}
	}
	return ""
}

func cabecalhosDe(gerente *elemento) map[string]string {
	cabecalhos := map[string]string{}
	for _, cabecalho := range gerente.buscarTodos("elementProp") {
		nome := cabecalho.propriedade("Header.name")
		valor := cabecalho.propriedade("Header.value")
		if nome != "" {
			cabecalhos[nome] = valor
		}
	}
	return cabecalhos
}

func fonteDeCSV(conjunto *elemento) FonteImportada {
	arquivo := conjunto.propriedade("filename")
	nome := strings.TrimSpace(conjunto.atributo("testname"))
	if nome == "" || strings.Contains(nome, " ") {
		nome = "dados"
	}
	consumo := "circular"
	if conjunto.propriedade("recycle") == "false" {
		consumo = "sequencial"
	}
	return FonteImportada{Nome: nome, Arquivo: arquivo, Consumo: consumo}
}

// Thread nao vira taxa: no JMeter cada thread so envia depois que a resposta
// anterior chegou, entao 50 threads viram 50/s se o alvo responde em 1 s e 5/s
// se responde em 10 s. Converter em silencio importaria a omissao coordenada
// junto com o cenario.
func avisosDeCarga(raiz *elemento) []string {
	var avisos []string
	for _, grupo := range raiz.buscarTodos("ThreadGroup") {
		threads := grupo.propriedade("ThreadGroup.num_threads")
		if threads == "" {
			continue
		}
		duracao := grupo.propriedade("ThreadGroup.duration")
		descricao := threads + " threads"
		if rampa := grupo.propriedade("ThreadGroup.ramp_time"); rampa != "" && rampa != "0" {
			descricao += ", rampa de " + rampa + "s"
		}
		if duracao != "" && duracao != "0" {
			descricao += ", " + duracao + "s de duracao"
		}
		avisos = append(avisos, fmt.Sprintf(
			"o grupo %q declara %s: numero de thread nao vira taxa de chegada, porque thread so envia depois da resposta anterior. "+
				"O bloco 'carga' ficou com um chute; troque pela taxa que voce quer sustentar (requisicoes por segundo)",
			grupo.atributo("testname"), descricao))
	}
	return avisos
}

func avisosDeCorrelacao(raiz *elemento) []string {
	var avisos []string
	for _, extrator := range raiz.buscarTodos("JSONPostProcessor") {
		variavel := extrator.propriedade("JSONPostProcessor.referenceNames")
		caminho := extrator.propriedade("JSONPostProcessor.jsonPathExprs")
		if variavel == "" {
			continue
		}
		avisos = append(avisos, fmt.Sprintf(
			"o .jmx captura %q de %q: declare no passo que produz o valor, como captura: { %s: %s }",
			variavel, caminho, variavel, caminho))
	}
	for _, extrator := range raiz.buscarTodos("RegexExtractor") {
		variavel := extrator.propriedade("RegexExtractor.refname")
		expressao := extrator.propriedade("RegexExtractor.regex")
		if variavel == "" {
			continue
		}
		avisos = append(avisos, fmt.Sprintf(
			"o .jmx captura %q por expressao regular: declare no passo que produz o valor, como captura: { %s: /%s/ }",
			variavel, variavel, expressao))
	}
	for _, assercao := range raiz.buscarTodos("ResponseAssertion") {
		nome := assercao.atributo("testname")
		avisos = append(avisos, fmt.Sprintf(
			"a assercao %q nao foi traduzida: todo passo saiu com 'verificar: { status: 200 }', ajuste o que era diferente disso", nome))
	}
	return avisos
}

func avisosDoQueFicouDeFora(raiz *elemento) []string {
	ignorados := map[string]int{}
	var contar func(*elemento)
	contar = func(atual *elemento) {
		nome := atual.XMLName.Local
		if nome != "" && !elementosTraduzidos[nome] {
			ignorados[nome]++
			return
		}
		for _, filho := range atual.Filhos {
			contar(filho)
		}
	}
	contar(raiz)

	if len(ignorados) == 0 {
		return nil
	}
	nomes := make([]string, 0, len(ignorados))
	total := 0
	for nome, quantidade := range ignorados {
		nomes = append(nomes, nome+" ("+strconv.Itoa(quantidade)+")")
		total += quantidade
	}
	sort.Strings(nomes)
	return []string{fmt.Sprintf(
		"%d elemento(s) do .jmx nao foram traduzidos e ficaram de fora do cenario: %s. "+
			"Confira se algum deles mudava o que era medido", total, strings.Join(nomes, ", "))}
}

func alvoMaisComum(alvos map[string]int) string {
	melhor, maior := "", 0
	for alvo, quantidade := range alvos {
		if quantidade > maior || (quantidade == maior && alvo < melhor) {
			melhor, maior = alvo, quantidade
		}
	}
	return melhor
}
