#!/usr/bin/env python3
"""Le os JSON brutos da bateria e imprime as tabelas em markdown.

Ponto com repeticao que colapsou nao vira media: a media misturaria execucao
valida com zero e esconderia justamente o que interessa.
"""
import json
import statistics
from pathlib import Path

BRUTO = Path(__file__).resolve().parent / "bruto"


def carregar(experimento):
    caminho = BRUTO / f"resumo-{experimento}.json"
    return json.loads(caminho.read_text()) if caminho.exists() else []


def sustentou_execucao(execucao):
    if execucao.get("colapsou") or not execucao.get("drenou"):
        return False
    alvo = execucao["taxa_alvo"]
    medidas = max(1, execucao["medidas"])
    return (execucao["taxa_efetiva"] >= 0.99 * alvo
            and execucao["erros"] == 0
            and execucao["despachos_atrasados"] / medidas < 0.01
            and execucao["desvio_de_agendamento_us"]["p99"] < 10_000)


def faixa(valores, casas=0):
    if not valores:
        return "—"
    mediana = statistics.median(valores)
    margem = (max(valores) - min(valores)) / 2
    return f"{mediana:,.{casas}f} ± {margem:,.{casas}f}".replace(",", ".")


def so_validas(agregado):
    return [e for e in agregado["execucoes"] if sustentou_execucao(e)]


def tabela_taxa(dados):
    linhas = ["| Taxa alvo | Prototipo | Repeticoes sustentadas | Taxa efetiva (reps validas) | Desvio p50 (us) | Desvio p99 (us) | Desvio max (us) | Erros (pior rep) | Pico em voo (pior rep) |",
              "|---|---|---|---|---|---|---|---|---|"]
    for agregado in dados:
        execucoes = agregado["execucoes"]
        validas = so_validas(agregado)
        linhas.append("| {} /s | {} | {}/{} | {} | {} | {} | {} | {} | {} |".format(
            f"{agregado['taxa_alvo']:,}".replace(",", "."),
            agregado["prototipo"],
            len(validas), len(execucoes),
            faixa([e["taxa_efetiva"] for e in validas], 1),
            faixa([e["desvio_de_agendamento_us"]["p50"] for e in validas]),
            faixa([e["desvio_de_agendamento_us"]["p99"] for e in validas]),
            faixa([e["desvio_de_agendamento_us"]["max"] for e in validas]),
            f"{max((e['erros'] for e in execucoes), default=0):,}".replace(",", "."),
            f"{max((e['pico_em_andamento'] for e in execucoes), default=0):,}".replace(",", ".")))
    return "\n".join(linhas)


def tabela_recursos(dados):
    linhas = ["| Taxa alvo | Prototipo | RSS repouso (MB) | RSS sob carga, pior rep (MB) | CPU (% de 1 nucleo) | Latencia corrigida p99 (us) | Latencia de servico p99 (us) | Delta corrigida-servico (us) |",
              "|---|---|---|---|---|---|---|---|"]
    for agregado in dados:
        validas = so_validas(agregado)
        pior_rss = max((e["rss_sob_carga_mb"] for e in agregado["execucoes"]), default=0)
        corrigida = [e["latencia_corrigida_us"]["p99"] for e in validas]
        servico = [e["latencia_de_servico_us"]["p99"] for e in validas]
        delta = [c - s for c, s in zip(corrigida, servico)]
        linhas.append("| {} /s | {} | {} | {} | {} | {} | {} | {} |".format(
            f"{agregado['taxa_alvo']:,}".replace(",", "."),
            agregado["prototipo"],
            faixa([e["rss_repouso_mb"] for e in agregado["execucoes"]], 1),
            f"{pior_rss:,.1f}".replace(",", "."),
            faixa([e["cpu_percentual_de_um_nucleo"] for e in validas], 1),
            faixa(corrigida),
            faixa(servico),
            faixa(delta)))
    return "\n".join(linhas)


def tabela_erros(dados):
    linhas = ["| Taxa alvo | Prototipo | Colapsos | Erros por classe |", "|---|---|---|---|"]
    for agregado in dados:
        classes = agregado.get("erros_por_classe", {})
        texto = ", ".join(f"`{classe}`: {quantidade:,}".replace(",", ".")
                          for classe, quantidade in list(classes.items())[:4]) or "nenhum"
        linhas.append("| {} /s | {} | {} | {} |".format(
            f"{agregado['taxa_alvo']:,}".replace(",", "."),
            agregado["prototipo"], agregado.get("colapsos", 0), texto))
    return "\n".join(linhas)


def tabela_concorrencia(dados):
    linhas = ["| Taxa alvo | Prototipo | Reps sustentadas | Pico em voo | Taxa efetiva | Desvio p99 (us) | RSS sob carga (MB) | Erros |",
              "|---|---|---|---|---|---|---|---|"]
    for agregado in dados:
        validas = so_validas(agregado)
        linhas.append("| {} /s | {} | {}/{} | {} | {} | {} | {} | {} |".format(
            f"{agregado['taxa_alvo']:,}".replace(",", "."),
            agregado["prototipo"], len(validas), len(agregado["execucoes"]),
            faixa([e["pico_em_andamento"] for e in validas]),
            faixa([e["taxa_efetiva"] for e in validas], 1),
            faixa([e["desvio_de_agendamento_us"]["p99"] for e in validas]),
            faixa([e["rss_sob_carga_mb"] for e in agregado["execucoes"]], 1),
            f"{max((e['erros'] for e in agregado['execucoes']), default=0):,}".replace(",", ".")))
    return "\n".join(linhas)


def inclinacao(pontos):
    n = len(pontos)
    if n < 2:
        return 0.0, 0.0
    media_x = sum(x for x, _ in pontos) / n
    media_y = sum(y for _, y in pontos) / n
    numerador = sum((x - media_x) * (y - media_y) for x, y in pontos)
    denominador = sum((x - media_x) ** 2 for x, _ in pontos)
    marginal = numerador / denominador if denominador else 0.0
    return marginal, media_y - marginal * media_x


def custo_marginal(dados):
    saida = []
    prototipos = sorted({a["prototipo"] for a in dados})
    for prototipo in prototipos:
        pontos = []
        for agregado in dados:
            if agregado["prototipo"] != prototipo:
                continue
            for execucao in so_validas(agregado):
                pontos.append((execucao["taxa_efetiva"],
                               execucao["cpu_percentual_de_um_nucleo"] / 100 * 1e9))
        marginal, base = inclinacao(pontos)
        saida.append((prototipo, len(pontos), marginal, base / 1e9))
    return saida


def main():
    startup_caminho = BRUTO / "resumo-startup.json"
    if startup_caminho.exists():
        startup = json.loads(startup_caminho.read_text())
        print("### Startup\n")
        print("| Prototipo | Startup (ms) | Amostras |")
        print("|---|---|---|")
        for prototipo, valores in startup.items():
            print(f"| {prototipo} | {valores['media']:.1f} ± {valores['margem']:.1f} | {valores['n']} |")
        print()

    for experimento, titulo in (("taxa", "Taxa de chegada"), ("gc", "Coletor de lixo (Java)")):
        dados = carregar(experimento)
        if not dados:
            continue
        print(f"### {titulo} — sustentacao e precisao do agendamento\n")
        print(tabela_taxa(dados))
        print(f"\n### {titulo} — recursos do gerador\n")
        print(tabela_recursos(dados))
        print(f"\n### {titulo} — colapso e erros\n")
        print(tabela_erros(dados))
        print(f"\n### {titulo} — custo marginal de CPU\n")
        print("| Prototipo | Execucoes validas | Custo marginal (us de CPU/req) | Piso do agendador (nucleos) |")
        print("|---|---|---|---|")
        for prototipo, pontos, marginal, piso in custo_marginal(dados):
            print(f"| {prototipo} | {pontos} | {marginal/1000:.1f} | {piso:.2f} |")
        print()

    concorrencia = carregar("concorrencia")
    if concorrencia:
        print("### Concorrencia — alvo com 1 s de latencia\n")
        print(tabela_concorrencia(concorrencia))


if __name__ == "__main__":
    main()
