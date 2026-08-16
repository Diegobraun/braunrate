package demo

import "fmt"

// The demo runs a file it wrote, instead of a Spec built in Go: principle 1 of
// the product says the scenario is the truth, and a demonstration that runs
// something with no file behind it teaches that a secret path exists — and
// leaves whoever liked the result with nothing to edit.
func healthyScenario(target string) string {
	return fmt.Sprintf(`# Escrito por 'braunrate demo'. E um cenario comum: aponte o alvo para o seu
# servico, edite os passos e rode com 'braunrate execute'.
nome: Demonstracao
alvo: %s

# O alvo embutido exige token, como uma API de verdade exigiria. Sem este bloco
# a execucao inteira toma 401 e sai invalida.
autenticacao:
  tipo: token
  obter:
    http: { metodo: POST, caminho: /auth/token, corpo: { usuario: ana } }
    captura: { token: $.access_token }

# taxa: quantas requisicoes por segundo o braunrate dispara. Ele dispara nesse
# ritmo esteja o alvo rapido ou lento, que e o que usuarios de verdade fazem.
carga:
  perfis:
    - patamar: { taxa: %s, durante: %s }

cenario:
  # Caminho fixo: toda requisicao vai ser identica, e o relatorio avisa que o
  # numero fica otimista. Para medir o servico, e nao o cache dele, troque por
  # /pedidos/${id} e declare de onde ${id} vem.
  - http: GET /pedidos/1
    nome: consultar pedido
    verificar: { status: 200 }

# criterio de aceite: se estourar, 'braunrate execute' sai com codigo 1 e o seu
# CI reprova.
slo:
  - global: { erros: < 0.1 }
`, target, rate, duration)
}

// The failing demo sends exactly the request the closed loop sends, with no
// authentication in the way: the whole point is that the two measurements
// differ because of when the request is counted, not because of what it asked
// for.
func freezingScenario(target string) string {
	return fmt.Sprintf(`# Escrito por 'braunrate demo --com-falha'. O alvo desta demonstracao trava de
# proposito no meio da execucao, como um GC longo ou um failover fariam.
nome: Demonstracao com falha
alvo: %s

carga:
  perfis:
    - patamar: { taxa: %s, durante: %s }

cenario:
  - http: GET /pedido
    nome: consultar pedido
    verificar: { status: 200 }

# criterio de aceite: este cenario existe para estourar. Numa ferramenta de
# laco fechado a mesma pausa passaria despercebida e o criterio aprovaria.
slo:
  - global: { erros: < 0.1 }
  - global: { p95: < 100ms }
`, target, rate, duration)
}
