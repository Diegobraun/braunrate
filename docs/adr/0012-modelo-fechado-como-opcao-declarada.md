# ADR 0012 — Modelo fechado como opcao declarada, nunca como padrao

## Contexto

A ferramenta nasceu com um so modelo de chegada, o aberto: a carga e uma taxa
declarada e o gerador insiste nela mesmo quando o alvo demora. E a unica forma
de medir sem omissao coordenada, e continua sendo o padrao.

Nem todo teste, porem, quer isso. Quem migra de JMeter tem cenario escrito em
numero de threads, e quem testa sistema com sessao (pool de conexao, licenca por
usuario, limite de assento) descreve a carga em usuarios simultaneos, nao em
requisicoes por segundo. Recusar o modelo fechado empurra essas pessoas de volta
para a ferramenta que mente sem avisar.

## Decisao

O modelo fechado existe, e declarado, e nunca e o padrao:

```yaml
carga:
  modelo: fechado
  usuarios: 200
  duracao: 5m
  intervalo_entre_iteracoes: 1s
```

Seis regras nao negociaveis:

1. **Sem latencia corrigida e sem delta de omissao coordenada.** No laco fechado
   nao existe instante agendado do qual contar. O campo `latencia_corrigida`
   fica **ausente** do documento, nao zerado: zero ali seria lido como "nenhum
   atraso escondido", que e exatamente o que este modelo nao pode afirmar.
2. **Aviso permanente em primeiro plano**, no terminal e no HTML, sempre, mesmo
   quando todos os numeros passam.
3. **`braunrate validate` avisa** ao ver `modelo: fechado`, explica a diferenca
   e mostra a taxa aproximada que aquele cenario produziria — em tres tempos de
   resposta, porque um numero so seria a promessa que este modelo nao cumpre.
4. **A comparacao recusa** aberto contra fechado, como ressalva bloqueante.
5. **Saturacao e invalidacao valem igual.** O que muda e o que deixa de existir:
   sem agendamento, nao ha despacho atrasado a medir, entao o corte de 1% nao se
   aplica. A verificacao de execucao curta passa a ser feita pela janela
   declarada.
6. **Teste dedicado** garante que o documento fechado nunca traz campo de
   latencia corrigida e que o aviso aparece nas duas saidas.

## Consequencias

A taxa deixa de ser algo que se pede e passa a ser resultado. O relatorio diz
isso na cara: no lugar de "atraso tipico para disparar", o bloco de
confiabilidade informa que a taxa efetiva foi consequencia do tempo de resposta
do alvo.

A prova esta no README, com o mesmo alvo congelado por 3 s medido dos dois
jeitos: **95% em 2,41 s no aberto contra 7,0 ms no fechado**. O fechado nao
errou uma conta — ele mediu com honestidade um evento que ele mesmo deixou de
provocar.

## Sobre o importador .jmx

O importador da Fase 6 ja resolveu o grupo de threads de outra forma: nao
converte para taxa, deixa um chute no bloco `carga` e avisa em texto que numero
de thread nao vira taxa de chegada. Fica como esta.

Importar direto para `modelo: fechado` seria a traducao mais fiel do arquivo, e
e justamente por isso que nao fazemos: quem importa um `.jmx` esta mudando de
ferramenta, e entregar o modelo fechado de volta importaria a omissao coordenada
junto com o cenario. O aviso empurra para o modelo aberto, que e o motivo de
trocar de ferramenta. Quem quiser o fechado declara — em uma linha.
