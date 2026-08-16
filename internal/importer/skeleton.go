package importer

import "strings"

// Skeleton exists because from an empty folder there was no path to a first
// scenario: every command takes a file and none created one. The comments in
// it are for someone reading this YAML for the first time, which is why they
// show the shape of the blocks nearly every scenario needs.
func Skeleton() string {
	return `# yaml-language-server: $schema=https://raw.githubusercontent.com/Diegobraun/braunrate/main/docs/braunrate.schema.json
# Com a extensao YAML do editor, a linha acima liga o autocompletar das chaves.

nome: Meu primeiro cenario
alvo: http://127.0.0.1:8080

# Taxa de chegada, em requisicoes por segundo. Nao e numero de usuarios: o
# gerador dispara na hora marcada mesmo que o alvo esteja lento, que e o que
# faz a medicao nao esconder travada.
carga:
  perfis:
    - rampa: { de: 1/s, ate: 20/s, durante: 30s }
    - patamar: { taxa: 20/s, durante: 1m }

cenario:
  - http: GET /pedidos/1
    nome: consultar pedido
    verificar: { status: 200 }

# Criterio de aceite. Sem este bloco a execucao roda e reporta, mas nunca falha.
slo:
  - consultar pedido: { p95: < 200ms }
  - global: { erros: < 1 }

# Dados que variam por iteracao. Um valor fixo faz o alvo responder de cache e
# o numero fica otimista; o relatorio declara a variedade que de fato aconteceu.
# dados:
#   assinantes: { arquivo: assinantes.csv, consumo: circular }
#
# cenario:
#   - http: GET /pedidos/${assinantes.id}

# Login uma vez, token reaproveitado pelas iteracoes seguintes.
# autenticacao:
#   tipo: token
#   obter:
#     http: { metodo: POST, caminho: /auth/token, corpo: { usuario: ana, senha: "${SENHA}" } }
#     captura: { token: $.access_token }

# O passo nao precisa ser HTTP. Estes sao os outros protocolos, e cada relatorio
# lista quais existem no binario que voce esta rodando.
` + commented(ProtocolShapes())
}

// ProtocolShapes is what the skeleton shows commented out. It lives apart from
// the text so a test can put it through the parser: a shape that does not parse
// teaches the wrong thing to exactly the person who has no other reference.
func ProtocolShapes() string {
	return `cenario:
  - graphql:
      consulta: |
        query ConsultarPedido($id: ID!) { pedido(id: $id) { status } }
      variaveis: { id: "${assinantes.id}" }

  - kafka: { topico: pedidos, chave: "${assinantes.id}", valor: { pedido: "${assinantes.id}" } }
  - amqp:  { fila: pedidos, corpo: { pedido: "${assinantes.id}" } }

  # Espera o efeito do passo anterior aparecer. A latencia dele tem a
  # granularidade da sondagem, e o relatorio diz isso.
  - aguardar:
      kafka: { topico: pedidos-processados }
      chave: "${assinantes.id}"
      timeout: 10s
`
}

func commented(text string) string {
	var out strings.Builder
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if line == "" {
			out.WriteString("#\n")
			continue
		}
		out.WriteString("# " + line + "\n")
	}
	return out.String()
}
