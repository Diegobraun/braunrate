# Principios de produto

Seis principios. Cada um nomeia um erro concreto do JMeter e a regra que adotamos para nao repeti-lo.

**Este documento e criterio de aceitacao.** Toda decisao de formato, de interface ou de mensagem ao usuario e checada contra ele, e o relatorio de cada fase declara se algo novo entrou em conflito com algum principio.

## 1. O cenario e a verdade, nao a interface

**Erro do JMeter**: o `.jmx` e serializacao de arvore de widgets, nao descricao de teste — por isso o diff e ilegivel e o merge e inviavel.

**Regra**: o YAML e a unica fonte de verdade. Qualquer interface futura le e escreve YAML, e nada mais. Nenhum campo pode existir apenas na interface. Se um recurso nao couber no YAML, ele nao existe.

## 2. Autoria e execucao no mesmo lugar

**Erro do JMeter**: monta-se na GUI, mas a propria documentacao manda rodar em modo nao-GUI. A ferramenta principal nao serve para o trabalho real.

**Regra**: qualquer interface dispara o mesmo motor, com o mesmo caminho de execucao do CLI, e sempre exibe o comando equivalente para copiar. Nunca existe "modo de montar" separado do "modo de rodar".

## 3. O usuario declara intencao, nao mecanica

**Erro do JMeter**: escopo de elemento, ordem de execucao, distincao entre pre-processor, post-processor e listener — conceitos de implementacao que viraram conceitos de usuario.

**Regra**: o QA declara o que quer (capturo este valor, espero este status, quero esta taxa). Onde isso acontece no ciclo de execucao e responsabilidade do motor. Nenhum conceito interno do braunrate deve aparecer no YAML ou nas mensagens.

## 4. O caminho comum cabe em uma tela

**Erro do JMeter**: dezenas de campos visiveis por elemento, quase todos irrelevantes para o caso comum.

**Regra**: o caso comum e curto e tem valor padrao sensato para tudo. Opcao avancada existe, mas nao ocupa espaco de quem nao precisa dela. Ao adicionar configuracao nova, o padrao deve funcionar sem que ninguem a declare.

## 5. Nunca comecar do zero

**Erro do JMeter**: folha em branco e arvore vazia. Na pratica todo mundo grava com proxy porque montar do nada e inviavel.

**Regra**: sempre existe um caminho de entrada — importar cURL, gravar trafego, partir de exemplo ou de assistente. Criar cenario a partir do vazio e o caso raro, nao o padrao.

## 6. Feedback antes da carga

**Erro do JMeter**: descobre-se que a correlacao quebrou depois de minutos de execucao.

**Regra**: montar e depurar acontece com um usuario e uma iteracao, com visibilidade total de requisicao, resposta, captura, variavel e assercao. Rodar carga e o ultimo passo, nunca o primeiro.

## Como isso se aplica hoje

| Principio | Onde ja vale | Onde ainda nao vale |
|---|---|---|
| 1 | YAML e a unica entrada do motor; DSL e importador produzem o mesmo modelo ([ADR 0002](adr/0002-modelo-de-cenario.md)); `import curl` escreve YAML, nao um formato proprio | — |
| 2 | so existe CLI; `debug` usa o mesmo motor de `execute`, e o fim da depuracao imprime o comando de carga equivalente; `import` imprime o `debug` seguinte | interface grafica nao existe; quando existir, exibe o comando equivalente |
| 3 | `captura`, `verificar`, `slo` e `aguardar` declaram intencao (o usuario diz qual mensagem espera, nao como consumir o topico); `debug` mostra a requisicao descrita pelo protocolo, nao a struct interna; `tipo de latencia` aparece como nota em portugues; em GraphQL o usuario cola a consulta e a ferramenta decide a chave de agregacao | — |
| 4 | tudo tem padrao: `modelo`, `nome`, `consumo`, `renovar_apos`, `verificar`, `caminho` do GraphQL, `acks`, `confirmar` e `timeout` da mensageria; o schema documenta cada chave no editor | — |
| 5 | `import curl` grava um cenario que ja roda; exemplos versionados em `examples/` | gravacao de trafego por proxy nao existe |
| 6 | `braunrate debug`: um usuario, uma iteracao, requisicao, resposta, captura e variavel visiveis, tambem em GraphQL | assercao ainda nao aparece passo a passo na depuracao quando passa |
