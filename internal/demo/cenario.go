package demo

import "fmt"

// The demo runs a file it wrote, instead of a Spec built in Go: principle 1 of
// the product says the scenario is the truth, and a demonstration that runs
// something with no file behind it teaches that a secret path exists — and
// leaves whoever liked the result with nothing to edit.
func healthyScenario(target string) string {
	return fmt.Sprintf(`# Escrito por 'braunrate demo'. E um cenário comum: aponte o alvo para o seu
# serviço, edite os passos e rode com 'braunrate execute'.
nome: Demonstração
alvo: %s

# O alvo embutido exige token, como uma API de verdade exigiria. Sem este bloco
# a execução inteira toma 401 e sai inválida.
autenticacao:
  tipo: token
  obter:
    http: { metodo: POST, caminho: /auth/token, corpo: { usuario: ana } }
    captura: { token: $.access_token }

# taxa: quantas requisições por segundo o braunrate dispara. Ele dispara nesse
# ritmo esteja o alvo rápido ou lento, que e o que usuários de verdade fazem.
carga:
  perfis:
    - patamar: { taxa: %s, durante: %s }

cenario:
  # Caminho fixo: toda requisição vai ser idêntica, e o relatório avisa que o
  # número fica otimista. Para medir o serviço, e não o cache dele, troque por
  # /pedidos/${id} e declare de onde ${id} vem.
  - http: GET /pedidos/1
    nome: consultar pedido
    verificar: { status: 200 }

# critério de aceite: se estourar, 'braunrate execute' sai com código 1 e o seu
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
# proposito no meio da execução, como um GC longo ou um failover fariam.
nome: Demonstração com falha
alvo: %s

carga:
  perfis:
    - patamar: { taxa: %s, durante: %s }

cenario:
  - http: GET /pedido
    nome: consultar pedido
    verificar: { status: 200 }

# critério de aceite: este cenário existe para estourar. Numa ferramenta de
# laço fechado a mesma pausa passaria despercebida e o critério aprovaria.
slo:
  - global: { erros: < 0.1 }
  - global: { p95: < 100ms }
`, target, rate, duration)
}
