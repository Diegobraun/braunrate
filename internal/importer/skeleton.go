package importer

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
`
}
