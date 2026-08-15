# ADR 0009 — A DSL nao reimplementa o YAML: os dois entram pelo mesmo lugar

- **Status**: aceito
- **Data**: 2026-08-16
- **Contexto de decisao**: Fase 6
- **Relacionados**: [ADR 0002](0002-formato-de-cenario.md), [ADR 0003](0003-modelo-de-execucao-e-metrica.md), [ADR 0004](0004-extensao-de-protocolo.md)

## Contexto

A promessa dos dois publicos ([ADR 0002](0002-formato-de-cenario.md)) e que migrar de YAML para Go nao seja reescrever. O jeito comum de entregar isso e escrever a DSL como uma segunda porta de entrada: uma API fluente que monta as mesmas estruturas, com os mesmos padroes, repetidos.

Repetidos e a palavra. Um timeout padrao aplicado num caminho e nao no outro, uma expressao de captura interpretada com uma regra ligeiramente diferente, uma validacao que so o YAML faz — cada um desses vira um cenario que mede uma coisa em YAML e outra em Go, com o relatorio afirmando que sao a mesma medicao. E o tipo de divergencia que ninguem encontra lendo codigo: os dois lados parecem certos separadamente.

## Decisao

**Nao existe interpretacao propria da DSL. Onde havia leitura presa a no de YAML, ela foi extraida para funcao sem no, e os dois caminhos chamam a mesma.**

1. **Interpretacao compartilhada no pacote `cenario`**: `MontarCaptura` (`$.campo`, `cabecalho:X-Id`, `/regex/`), `MontarComparacao` (`> 10`, `existe`, `contem`), `MontarRegraDeSLO` (`< 150ms`, `> 500/s`), `Interpolar` e `ExpandirDoAmbiente`. O leitor de YAML virou uma casca fina que so acrescenta linha e coluna ao erro.
2. **Padrao e validacao de protocolo tambem num lugar so**: cada protocolo expoe `Padrao()` e `Validar()` (ou `Finalizar()`, no GraphQL, que extrai o nome da operacao). O `Decodificar` do YAML e o construtor da DSL chamam os mesmos. Protocolo novo entra equivalente sem escrever nada a mais.
3. **A equivalencia e travada por teste, nao por revisao.** Cada caso tem o YAML e o codigo Go que deveriam produzir o mesmo cenario, e o teste compara a estrutura inteira — nao um resumo, nao uma amostra. So o numero da linha e ignorado: e posicao no arquivo, e nao existe em codigo Go.
4. **A cobertura do teste tambem e travada.** Um teste falha se um protocolo registrado, uma chave de topo do YAML, uma origem de captura, um tipo de assercao, um perfil de carga, um tipo de autenticacao ou uma politica de consumo nao aparecer em nenhum caso de equivalencia. Sem isso, o que fosse acrescentado depois nasceria sem equivalencia verificada e a promessa valeria so para o que ja existia.
5. **A DSL nao acrescenta capacidade que o YAML nao tem.** Ela acrescenta o que so codigo da: laco, condicao, dado vindo de um sistema proprio. O cenario resultante e o mesmo tipo que o YAML produz e vai para o mesmo motor.

## Alternativas descartadas

- **DSL gerando texto YAML e reaproveitando o parser**: equivalencia de graca, mas o erro sairia como "linha 14 do arquivo que voce nao escreveu", e o autocompletar do editor Go — que e a razao de existir da DSL — some.
- **Duas implementacoes com teste de amostra**: e o estado que a maioria das ferramentas aceita. Amostra passa e a divergencia mora no caso que ninguem escreveu.
- **Gerar a DSL a partir do schema**: resolveria a repeticao, custa um gerador de codigo e entrega uma API que ninguem desenhou.

## Consequencias

- Mudanca de padrao (um timeout, uma regra de captura) precisa ser feita uma vez, e vale para os dois publicos automaticamente.
- Acrescentar chave nova ao YAML sem acrescentar na DSL quebra o build — de proposito.
- A DSL herda as validacoes que ensinam: operacao GraphQL sem nome, `aguardar` sem correlacao e passo Kafka sem valor recusam do mesmo jeito, com a mesma mensagem.
- Fica pendente: comparar tambem a **execucao** dos dois caminhos alem da estrutura (hoje ha um teste que roda o par gemeo e compara chaves de agregacao e veredito, mas nao a distribuicao inteira).
