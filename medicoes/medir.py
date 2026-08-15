#!/usr/bin/env python3
"""Bateria de medicao dos prototipos da Fase 0.

Roda o mesmo experimento contra o mesmo alvo para os dois prototipos e
grava um JSON bruto por execucao em medicoes/bruto/.
"""
import json
import os
import re
import signal
import statistics
import subprocess
import sys
import threading
import time
from pathlib import Path

RAIZ = Path(__file__).resolve().parent.parent
BRUTO = RAIZ / "medicoes" / "bruto"
ALVO_BINARIO = RAIZ / "prototipos" / "alvo" / "alvo"
GO_BINARIO = RAIZ / "prototipos" / "go" / "braunrate-proto-go"
JAVA_CLASSES = RAIZ / "prototipos" / "java" / "classes"
JAVA_JAR = RAIZ / "prototipos" / "java" / "lib" / "HdrHistogram-2.2.2.jar"
PORTA = 8471


def rss_kb(pid):
    try:
        saida = subprocess.run(["ps", "-o", "rss=", "-p", str(pid)],
                               capture_output=True, text=True, timeout=5).stdout.strip()
        return int(saida) if saida else 0
    except Exception:
        return 0


def subir_alvo(latencia_ms):
    processo = subprocess.Popen(
        [str(ALVO_BINARIO), f"-porta={PORTA}", f"-latencia={latencia_ms}ms"],
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    for _ in range(100):
        try:
            subprocess.run(["curl", "-sf", f"http://127.0.0.1:{PORTA}/saude"],
                           capture_output=True, timeout=2, check=True)
            return processo
        except Exception:
            time.sleep(0.1)
    raise RuntimeError("alvo nao subiu")


def comando(prototipo, taxa, duracao, aquecimento, espera_antes):
    url = f"http://127.0.0.1:{PORTA}/pedido"
    if prototipo == "go":
        return [str(GO_BINARIO), f"-alvo={url}", f"-taxa={taxa}",
                f"-duracao={duracao}s", f"-aquecimento={aquecimento}s",
                f"-espera-antes={espera_antes}s"]
    return ["java", "-cp", f"{JAVA_CLASSES}:{JAVA_JAR}", "PrototipoJava",
            f"-alvo={url}", f"-taxa={taxa}", f"-duracao={duracao}",
            f"-aquecimento={aquecimento}", f"-espera-antes={espera_antes}"]


def medir_startup(prototipo, repeticoes):
    """Tempo de parede entre exec e o prototipo estar apto a gerar carga.

    Medido fora do harness em Python para nao contaminar com o custo do pipe.
    """
    alvo = subir_alvo(5)
    tempos = []
    try:
        for _ in range(repeticoes):
            argumentos = comando(prototipo, 1, 0, 0, 0)
            inicio = time.monotonic()
            subprocess.run(argumentos, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=60)
            tempos.append((time.monotonic() - inicio) * 1000)
    finally:
        alvo.send_signal(signal.SIGTERM)
        alvo.wait(timeout=10)
    return tempos


def executar(prototipo, taxa, latencia_ms, duracao=15, aquecimento=5, espera_antes=3):
    alvo = subir_alvo(latencia_ms)
    time.sleep(0.5)
    try:
        inicio = time.monotonic()
        processo = subprocess.Popen(comando(prototipo, taxa, duracao, aquecimento, espera_antes),
                                    stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        while True:
            linha = processo.stderr.readline()
            if not linha:
                raise RuntimeError(f"{prototipo} morreu antes de ficar pronto")
            if "PRONTO" in linha:
                break
        startup_ms = (time.monotonic() - inicio) * 1000
        time.sleep(0.7)
        rss_repouso = rss_kb(processo.pid)

        pico = {"valor": rss_repouso}
        parar = threading.Event()

        def amostrar():
            while not parar.is_set():
                atual = rss_kb(processo.pid)
                if atual > pico["valor"]:
                    pico["valor"] = atual
                parar.wait(0.25)

        amostrador = threading.Thread(target=amostrar, daemon=True)
        amostrador.start()
        saida, _ = processo.communicate(timeout=duracao + aquecimento + 180)
        parar.set()
        amostrador.join(timeout=2)

        resultado = json.loads(saida)
        resultado["startup_ms"] = round(startup_ms, 1)
        resultado["rss_repouso_mb"] = round(rss_repouso / 1024, 1)
        resultado["rss_sob_carga_mb"] = round(pico["valor"] / 1024, 1)
        resultado["latencia_do_alvo_ms"] = latencia_ms
        return resultado
    finally:
        alvo.send_signal(signal.SIGTERM)
        alvo.wait(timeout=10)
        time.sleep(0.4)


def resumo(valores):
    if not valores:
        return {}
    media = statistics.fmean(valores)
    return {"media": round(media, 1), "min": round(min(valores), 1), "max": round(max(valores), 1),
            "margem": round((max(valores) - min(valores)) / 2, 1), "n": len(valores)}


def main():
    BRUTO.mkdir(parents=True, exist_ok=True)
    experimento = sys.argv[1] if len(sys.argv) > 1 else "taxa"
    repeticoes = int(os.environ.get("REPETICOES", "3"))

    if experimento == "startup":
        saida = {}
        for prototipo in ("java", "go"):
            tempos = medir_startup(prototipo, max(repeticoes, 5))
            saida[prototipo] = {"amostras_ms": [round(t, 1) for t in tempos], **resumo(tempos)}
        (BRUTO / "resumo-startup.json").write_text(json.dumps(saida, indent=2))
        print(json.dumps(saida, indent=2))
        return

    if experimento == "taxa":
        casos = [(t, 5) for t in (1000, 5000, 10000, 20000, 30000, 40000, 60000)]
    elif experimento == "concorrencia":
        casos = [(t, 1000) for t in (1000, 5000, 10000, 20000, 50000)]
    else:
        raise SystemExit("experimento deve ser 'taxa', 'concorrencia' ou 'startup'")

    tudo = []
    for taxa, latencia in casos:
        for prototipo in ("java", "go"):
            execucoes = []
            for repeticao in range(repeticoes):
                print(f"[{experimento}] {prototipo} taxa={taxa} latencia={latencia}ms rep={repeticao+1}",
                      file=sys.stderr, flush=True)
                execucoes.append(executar(prototipo, taxa, latencia))
            agregado = {
                "experimento": experimento,
                "prototipo": prototipo,
                "taxa_alvo": taxa,
                "latencia_do_alvo_ms": latencia,
                "taxa_efetiva": resumo([e["taxa_efetiva"] for e in execucoes]),
                "startup_ms": resumo([e["startup_ms"] for e in execucoes]),
                "rss_repouso_mb": resumo([e["rss_repouso_mb"] for e in execucoes]),
                "rss_sob_carga_mb": resumo([e["rss_sob_carga_mb"] for e in execucoes]),
                "cpu_ns_por_requisicao": resumo([e["cpu_ns_por_requisicao"] for e in execucoes]),
                "cpu_percentual_de_um_nucleo": resumo([e["cpu_percentual_de_um_nucleo"] for e in execucoes]),
                "desvio_p50_us": resumo([e["desvio_de_agendamento_us"]["p50"] for e in execucoes]),
                "desvio_p99_us": resumo([e["desvio_de_agendamento_us"]["p99"] for e in execucoes]),
                "desvio_max_us": resumo([e["desvio_de_agendamento_us"]["max"] for e in execucoes]),
                "latencia_corrigida_p99_us": resumo([e["latencia_corrigida_us"]["p99"] for e in execucoes]),
                "latencia_de_servico_p99_us": resumo([e["latencia_de_servico_us"]["p99"] for e in execucoes]),
                "pico_em_andamento": resumo([e["pico_em_andamento"] for e in execucoes]),
                "erros": resumo([e["erros"] for e in execucoes]),
                "despachos_atrasados": resumo([e["despachos_atrasados"] for e in execucoes]),
                "execucoes": execucoes,
            }
            tudo.append(agregado)
            destino = BRUTO / f"{experimento}-{prototipo}-{taxa}.json"
            destino.write_text(json.dumps(agregado, indent=2))

    (BRUTO / f"resumo-{experimento}.json").write_text(json.dumps(tudo, indent=2))
    print(json.dumps([{k: v for k, v in a.items() if k != "execucoes"} for a in tudo], indent=2))


if __name__ == "__main__":
    main()
