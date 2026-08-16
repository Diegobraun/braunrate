# ADR 0005 — Identidade: um token para a execucao inteira, por enquanto

- **Status**: aceito
- **Data**: 2026-08-15
- **Contexto de decisao**: Fase 2.5, apos revisao da Fase 2
- **Relacionados**: [ADR 0003](0003-modelo-de-execucao-e-metrica.md), [principios de produto](../principios-de-produto.md) §4

## Contexto

No modelo de chegada aberta nao existe usuario virtual de vida longa: cada chegada agendada e uma iteracao independente. A implementacao da Fase 2 obtem **um token para a execucao inteira** e o renova quando `renovar_apos` vence.

Isso resolve o caso comum com custo minimo e sem configuracao. E **nao reproduz producao**:

- **Cache por token**: se o alvo guarda resposta por identidade, a segunda iteracao em diante mede cache, nao o servico.
- **Rate limit por token**: um unico token concentra toda a carga numa cota que em producao estaria distribuida — o resultado pode ficar pessimista, ou o teste pode falhar por 429 que nao aconteceria.
- **Sharding por usuario**: se o alvo roteia por identidade, toda a carga cai num shard so. O resultado fica **otimista** quando o shard sozinho da conta, e enganoso sempre.

A alternativa — obter token por iteracao — e pior: mediria o servico de autenticacao junto, e a 300 iteracoes por segundo geraria 300 logins por segundo, o que nenhum ambiente real aceita.

## Decisao

**Mantemos o token unico por execucao como padrao, e declaramos a limitacao onde ela pode enganar alguem.**

Tres obrigacoes que vem junto:

1. **O relatorio declara.** Toda execucao com autenticacao imprime: *"Autenticacao obtida uma vez (ou N vezes) e reaproveitada por todas as jornadas. Se o alvo tiver cache, rate limit ou sharding por token, este numero fica otimista."*
2. **O README declara**, na secao de limitacoes conhecidas, junto com a friccao de protocolo compilado.
3. **A evolucao esta prevista e nomeada**, para nao virar reescrita:
   - **`pool de tokens`**: o cenario declara N identidades (de CSV ou de geracao), o motor obtem um token por identidade no inicio e distribui entre as iteracoes com a mesma politica de consumo dos dados (`circular`, `unico_por_usuario`, ...). Reaproveita `dados` e `autenticacao`, sem conceito novo para o usuario.
   - **`token por usuario virtual`**: cada iteracao obtem o proprio token. So faz sentido em cenario de baixa taxa; entra como opcao explicita, nunca como padrao.

A forma esperada no YAML, quando entrar — nenhum conceito interno novo, so uma chave a mais no bloco que ja existe:

```yaml
dados:
  identidades: { arquivo: dados/usuarios.csv, consumo: unico_por_usuario }

autenticacao:
  tipo: token
  identidades: identidades        # sem esta linha, continua um token para tudo
  obter:
    http: { metodo: POST, caminho: /auth/token, corpo: { usuario: "${identidades.usuario}", senha: "${identidades.senha}" } }
    captura: { token: $.access_token }
```

## Alternativas descartadas

- **Token por iteracao como padrao**: mede o servico de autenticacao junto e gera carga irreal de login.
- **Nao declarar a limitacao**: e o antipadrao que o projeto existe para nao cometer — numero que parece bom porque a medicao foi generosa.
- **Bloquear autenticacao ate ter pool**: negaria o caso comum, que e legitimo e cobre a maioria dos cenarios de leitura.

## Consequencias

- O bloco de ambiente do relatorio passa a ter uma linha sobre identidade, e o documento de resultado ja carrega `obtencoes_de_autenticacao`.
- Quando o pool entrar, o formato de resultado ganha a quantidade de identidades distintas — campo novo, sem quebrar o formato.
- Comparacao entre execucoes precisa considerar que duas execucoes com quantidade diferente de identidades nao sao comparaveis; isso entra na Fase 7.
