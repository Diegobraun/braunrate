# ADR 0021 — Credencial de sessão na interface, sem tocar o arquivo

- **Status**: aceito
- **Data**: 2026-08-17
- **Contexto de decisao**: uso real da interface web contra um alvo autenticado
- **Relacionados**: [ADR 0005](0005-identidade-e-token.md), [ADR 0018](0018-interface-como-editor-do-arquivo.md), [principios de produto](../principios-de-produto.md)

## Contexto

A interface web existe para o público que não escreve código — o QA que hoje usa o JMeter. Mas havia um passo que só o teclado do desenvolvedor alcança: **dar valor à credencial**.

O cenário declara `header: "Authorization: Bearer ${TOKEN}"`, e a única forma de preencher `${TOKEN}` era a variável de ambiente na linha que sobe o servidor: `TOKEN=... braunrate ui`. Quem só abre a tela não faz isso. E o caminho que essa pessoa tentaria — colar o Bearer direto no YAML — é justamente o que a ferramenta recusa na validação, com razão: o arquivo vai para o repositório, e segredo no arquivo é segredo publicado.

O resultado: a superfície "sem código" tinha um passo obrigatório de código, e o único atalho aparente estava bloqueado. A pessoa fica sem rodar.

Isso não apareceu na avaliação em três olhares porque quem escreveu o percurso sabia exportar variável de ambiente. A pessoa não sabe.

## Decisao

**A interface aceita o valor de uma variável referenciada no cenário, guardado só na memória do servidor pela duração da sessão, injetado no disparo exatamente onde a variável de ambiente entraria.** O arquivo continua com `${TOKEN}` — nunca o literal.

A regra de credencial fica inteira porque a distinção do projeto se mantém: **o arquivo diz de onde o valor vem, não qual é.** Variável de ambiente e campo-de-sessão são os dois "de onde"; o "qual" nunca toca o disco. Concretamente:

1. **O arquivo nunca muda.** O campo de sessão não é gravado no YAML; a recusa de credencial literal continua valendo, e colar o Bearer no arquivo continua sendo recusado.
2. **O valor vive na memória do processo** e some quando o servidor reinicia — o mesmo ciclo de vida de uma variável de ambiente num shell. Não há armazenamento persistente de segredo, e é assim de propósito.
3. **A saída não mostra segredo.** O valor de sessão passa pelo mesmo corte que já existe: terminal, HTML, JSON e depuração mostram `Bearer eyJhbG…`, nunca o token inteiro. A leitura de volta (`GET /environment`) devolve só os **nomes** preenchidos, nunca os valores.
4. **Só em `127.0.0.1`.** A interface já se planta no loopback e avisa que expor noutra interface é uma decisão à parte. O valor cruza HTTP em claro, e por isso só no loopback.

A injeção acontece num ponto só, `ExecuteSpec`, que é o caminho tanto do YAML quanto do cenário em Go — o mesmo veredito para os dois, como o [ADR 0009](0009-equivalencia-entre-yaml-e-dsl.md) exige.

## Alternativas descartadas

- **Colar o Bearer no YAML**: quebra a regra que o projeto existe para ter — segredo no arquivo é segredo versionado. É o que a validação recusa, e continuará recusando.
- **Guardar o segredo em disco ou no keychain**: cria um cofre de segredos, uma responsabilidade nova e uma superfície de vazamento que a ferramenta não tem hoje. A postura do projeto é não guardar segredo; um campo de sessão que morre no restart não guarda nada.
- **Só variável de ambiente**: é o que exclui o não-programador, que é a razão desta decisão.
- **Segredo em parâmetro de consulta**: colocaria a credencial na URL, que vai para log e histórico. O projeto já proíbe dado sensível em query.

## O que reabre esta decisão

- **Expor a interface além do loopback.** O valor em claro sobre HTTP só é aceitável em `127.0.0.1`. Servir noutra interface exige autenticação e TLS antes, e aí este caminho é revisto junto.
- **Precisar de segredo persistente entre reinícios.** Se algum dia isso se justificar, é um cofre de verdade e um ADR próprio — não uma extensão silenciosa deste campo.
