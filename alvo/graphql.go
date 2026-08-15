package alvo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type pedidoGraphQL struct {
	Consulta      string         `json:"query"`
	OperationName string         `json:"operationName"`
	Variaveis     map[string]any `json:"variables"`
}

// O alvo devolve erro de GraphQL com status 200 de proposito: e assim que o
// erro chega em producao, e e exatamente o caso que uma ferramenta que so olha
// o status HTTP contabiliza como sucesso.
func (s *Servidor) tratarGraphQL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var pedido pedidoGraphQL
	if err := json.NewDecoder(r.Body).Decode(&pedido); err != nil {
		responderGraphQL(w, "", `{"errors":[{"message":"corpo nao e JSON","extensions":{"code":"BAD_REQUEST"}}]}`)
		return
	}

	operacao := pedido.OperationName
	if operacao == "" {
		operacao = nomeDaOperacao(pedido.Consulta)
	}
	identificador := texto(pedido.Variaveis["id"])

	switch operacao {
	case "ConsultarPedido":
		if strings.HasSuffix(identificador, "7") {
			responderGraphQL(w, operacao, fmt.Sprintf(
				`{"data":{"pedido":null},"errors":[{"message":"pedido %s nao encontrado","path":["pedido"],"extensions":{"code":"NOT_FOUND"}}]}`,
				identificador))
			return
		}
		responderGraphQL(w, operacao, fmt.Sprintf(
			`{"data":{"pedido":{"id":%q,"status":"ABERTO","ultimaFatura":{"id":"f-%s","valor":199.90,"status":"ABERTA"}}}}`,
			identificador, identificador))
	case "PagarFatura":
		fatura := texto(pedido.Variaveis["fatura"])
		responderGraphQL(w, operacao, fmt.Sprintf(
			`{"data":{"pagarFatura":{"id":%q,"status":"PAGA","pagoEm":"2026-08-15T00:00:00Z"}}}`, fatura))
	default:
		responderGraphQL(w, operacao, fmt.Sprintf(
			`{"errors":[{"message":"operacao %q nao existe no schema","extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}}]}`, operacao))
	}
}

func responderGraphQL(w http.ResponseWriter, operacao, corpo string) {
	w.Header().Set("Content-Type", "application/json")
	if operacao != "" {
		w.Header().Set("X-Operacao", operacao)
	}
	fmt.Fprint(w, corpo)
}

func nomeDaOperacao(consulta string) string {
	campos := strings.Fields(consulta)
	for indice, campo := range campos {
		if campo != "query" && campo != "mutation" {
			continue
		}
		if indice+1 >= len(campos) {
			return ""
		}
		nome, _, _ := strings.Cut(campos[indice+1], "(")
		nome, _, _ = strings.Cut(nome, "{")
		return nome
	}
	return ""
}

func texto(valor any) string {
	if valor == nil {
		return ""
	}
	return fmt.Sprint(valor)
}
