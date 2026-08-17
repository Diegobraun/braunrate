# braunrate

```dobra
lema: Teste de carga que não mente sobre o próprio resultado.
resumo: Quando o sistema trava, a maioria das ferramentas para de medir junto — e o relatório sai bonito. O braunrate continua medindo, e mostra o que aconteceu.
comando: braunrate demo
acao: Baixar | instalacao.html
acao: Ver no GitHub | https://github.com/Diegobraun/braunrate
ficha: binário único | sem runtime para instalar | cenário em YAML versionado
prova: Mesmo serviço. Mesma travada de 1 segundo.
lado: Ferramenta de laço fechado | 3,7 ms | "está tudo bem"
lado: braunrate | 983,0 ms | "o usuário esperou 983 ms"
saldo: 979,4 ms que a outra ferramenta não contou.
```

## Começar

Se você nunca fez teste de carga, o caminho mais curto é um comando, sem arquivo
nenhum antes:

```bash
braunrate demo
```

Ainda não tem o binário? [Instalação](instalacao.html) é baixar um arquivo e
descompactar; não há runtime para instalar.

A demonstração sobe um serviço de mentira na sua máquina, roda um cenário contra
ele e explica cada número enquanto eles aparecem. Depois,
[Primeiros 15 minutos](primeiros-15-minutos.html) leva do zero até o primeiro
relatório do seu próprio serviço.

| Se você quer | Vá para |
|---|---|
| ver a ferramenta funcionando agora | `braunrate demo` |
| medir o seu serviço | [Primeiros 15 minutos](primeiros-15-minutos.html) |
| entender o que os números querem dizer | [Conceitos](conceitos.html) |
| escrever o cenário | [Referência do cenário](referencia.html) |
| resolver um erro na tela | [Solução de problemas](problemas.html) |

## Três provas

As três medições abaixo são execuções reais, e cada uma expõe uma forma
diferente de um teste de carga produzir um número que não descreve o sistema.

| Prova | Número | O que fica escondido |
|---|---|---|
| Alvo congelado por 1 s | 983,0 ms contra 3,7 ms | **Omissão coordenada**: laço fechado para de enviar quando o alvo trava, e a espera some da conta |
| [GraphQL com erro em 200](protocolos.html#graphql) | 406 erros em 2.844 respostas, todas com status 200 | **Erro classificado por status**: quem lê o código HTTP reporta 0% de erro e critério de aceite verde |
| [Cadeia assíncrona](protocolos.html#kafka-e-rabbitmq) | 1,2 ms para produzir contra 3,96 s de jornada | **Medir só a produção**: o broker aceita rápido, e o efeito que o usuário espera chega segundos depois |

### A primeira, em detalhe

O alvo de teste embutido congela por 1 s no meio da execução. Mesma pausa, mesmo
alvo, dois modelos de medição:

| Modelo | 99% das respostas em até | Amostras |
|---|---|---|
| **braunrate (chegada aberta, tempo contado do instante agendado)** | **983,0 ms** | 600 |
| Laço fechado (um usuário virtual em sequência, como JMeter e Locust medem) | 3,7 ms | 722 |

São 979,4 ms escondidos pelo laço fechado. Ele não erra por defeito: quando o
alvo trava, ele simplesmente para de enviar, e as requisições que deveriam ter
partido nunca entram na conta. Essa é a omissão coordenada.

A comparação é um teste automatizado que roda no CI a cada push. Se a medição
deixar de ser honesta, o build quebra:

```
$ go test ./internal/selfcheck/... -v
=== RUN   TestClosedLoopWouldHideThePauseOpenModelShows
    mesma pausa de 1s no mesmo alvo:
      modelo aberto (braunrate): p99 983.0 ms sobre 600 amostras
      laço fechado:              p99 3.7 ms sobre 722 amostras
      omissão coordenada: 979.4 ms escondidos pelo laço fechado
--- PASS: TestClosedLoopWouldHideThePauseOpenModelShows (6.01s)
```

Para ver a mesma coisa acontecer na sua máquina, sem clonar o repositório:

```bash
braunrate demo --with-failure
```

## Para quem é

**QA que não programa.** O caso comum é um arquivo YAML de dez linhas, com valor
padrão para tudo o que não foi declarado. Sempre existe um caminho de entrada —
importar um `curl`, gravar navegando, partir de um exemplo — porque folha em
branco é o motivo pelo qual ninguém monta cenário do zero.

**Time que versiona o teste junto do serviço.** O cenário é um arquivo de texto
que dá diff legível e merge possível, o mesmo motor roda no terminal e no CI, e o
critério de aceite vira código de saída sem cola no meio.

## Princípios

1. **Medição honesta por padrão.** Modelo de chegada aberto; tempo de resposta
   contado a partir do instante em que a requisição *deveria* ter partido; HDR
   histogram; aviso explícito quando o gerador não sustentou a taxa. A omissão
   coordenada é a falha que faz um teste passar com 99% em até 47 ms enquanto a
   produção sofre 1,8 s.
2. **Dois públicos, um motor.** YAML declarativo para o caso comum, DSL em Go
   para o complexo — mesmo motor, mesmas métricas, sem reescrita ao migrar.
3. **Cenário de negócio, não só requisição.** GraphQL medido por operação; Kafka
   e RabbitMQ com modelo de métrica próprio; passo `aguardar` para medir a cadeia
   assíncrona ponta a ponta.

## Escopo

**Dentro.** HTTP/HTTPS e REST; GraphQL de primeira classe; Kafka e RabbitMQ
(produzir e consumir); passo `aguardar` com timeout; correlação, variáveis e
fluxo de autenticação; CSV com política de consumo e geração sintética com
semente; perfis de carga e modelo fechado declarado; critério de aceite com
código de saída; relatório HTML autocontido, JSON, CSV e resumo de terminal;
comparação entre execuções; importador de `.jmx` para o subconjunto comum;
gravador de tráfego HTTP; modo servidor local sem lógica própria; autenticação de
broker, sempre com a credencial fora do arquivo.

**Fora.** Motor de browser real; nuvem gerenciada, dashboard multiusuário, conta
de time; agendamento e persistência além dos arquivos; LDAP, FTP, SMTP, JMS
clássico; competir em taxa bruta com wrk; execução distribuída na v1.

**Limitações conhecidas**, com o motivo em [Decisões](decisoes.html): protocolo
fora da lista exige mudança neste repositório; um único token para a execução
inteira; e o tempo dos passos seguintes ao primeiro é tempo de serviço, não tempo
corrigido — a leitura honesta da jornada está no bloco "A jornada inteira".
