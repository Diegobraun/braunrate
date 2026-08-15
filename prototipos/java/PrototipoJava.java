import com.sun.management.OperatingSystemMXBean;
import java.lang.management.ManagementFactory;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.HashMap;
import java.util.Map;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicLong;
import java.util.concurrent.locks.LockSupport;
import org.HdrHistogram.ConcurrentHistogram;
import org.HdrHistogram.Histogram;

public class PrototipoJava {

    static final long MAIOR_LATENCIA_US = 300_000_000L;

    static final ConcurrentHistogram latenciaCorrigida = new ConcurrentHistogram(1, MAIOR_LATENCIA_US, 3);
    static final ConcurrentHistogram latenciaDeServico = new ConcurrentHistogram(1, MAIOR_LATENCIA_US, 3);
    static final ConcurrentHistogram desvioDeAgendamento = new ConcurrentHistogram(1, MAIOR_LATENCIA_US, 3);

    static final AtomicLong enviadas = new AtomicLong();
    static final AtomicLong concluidas = new AtomicLong();
    static final AtomicLong erros = new AtomicLong();
    static final AtomicLong despachosAtrasados = new AtomicLong();
    static final AtomicInteger emAndamento = new AtomicInteger();
    static final AtomicInteger picoEmAndamento = new AtomicInteger();

    public static void main(String[] argumentos) throws Exception {
        Map<String, String> opcoes = lerOpcoes(argumentos);
        String alvo = opcoes.getOrDefault("alvo", "http://127.0.0.1:8080/pedido");
        long taxa = Long.parseLong(opcoes.getOrDefault("taxa", "1000"));
        long duracaoSegundos = Long.parseLong(opcoes.getOrDefault("duracao", "10"));
        long aquecimentoSegundos = Long.parseLong(opcoes.getOrDefault("aquecimento", "2"));
        long esperaAntes = Long.parseLong(opcoes.getOrDefault("espera-antes", "2"));
        long limiarAtrasoUs = Long.parseLong(opcoes.getOrDefault("limiar-atraso-us", "10000"));

        HttpClient cliente = HttpClient.newBuilder()
                .version(HttpClient.Version.HTTP_1_1)
                .connectTimeout(Duration.ofSeconds(5))
                .executor(Executors.newVirtualThreadPerTaskExecutor())
                .build();

        HttpRequest requisicao = HttpRequest.newBuilder(URI.create(alvo))
                .timeout(Duration.ofSeconds(30))
                .GET()
                .build();

        cliente.send(requisicao, HttpResponse.BodyHandlers.ofString());

        System.err.println("PRONTO");
        Thread.sleep(esperaAntes * 1000);

        OperatingSystemMXBean sistema = (OperatingSystemMXBean) ManagementFactory.getOperatingSystemMXBean();
        ExecutorService executor = Executors.newVirtualThreadPerTaskExecutor();

        long intervaloNanos = 1_000_000_000L / taxa;
        long inicio = System.nanoTime();
        long fimDoAquecimento = inicio + aquecimentoSegundos * 1_000_000_000L;
        long fim = fimDoAquecimento + duracaoSegundos * 1_000_000_000L;
        long cpuNoInicio = sistema.getProcessCpuTime();
        long relogioNoInicio = 0;
        boolean medindo = false;
        long indice = 0;

        while (true) {
            long agendado = inicio + indice * intervaloNanos;
            if (agendado >= fim) {
                break;
            }
            dormirAte(agendado);
            long despacho = System.nanoTime();
            boolean valeMedir = despacho >= fimDoAquecimento;
            if (valeMedir && !medindo) {
                medindo = true;
                cpuNoInicio = sistema.getProcessCpuTime();
                relogioNoInicio = despacho;
            }
            long atrasoUs = Math.max(0, (despacho - agendado) / 1000);
            if (valeMedir) {
                desvioDeAgendamento.recordValue(Math.max(1, atrasoUs));
                if (atrasoUs > limiarAtrasoUs) {
                    despachosAtrasados.incrementAndGet();
                }
            }
            enviadas.incrementAndGet();
            int atuais = emAndamento.incrementAndGet();
            picoEmAndamento.accumulateAndGet(atuais, Math::max);

            executor.submit(() -> {
                long envio = System.nanoTime();
                try {
                    HttpResponse<String> resposta = cliente.send(requisicao, HttpResponse.BodyHandlers.ofString());
                    long termino = System.nanoTime();
                    if (resposta.statusCode() != 200) {
                        erros.incrementAndGet();
                    } else if (valeMedir) {
                        latenciaCorrigida.recordValue(Math.max(1, (termino - agendado) / 1000));
                        latenciaDeServico.recordValue(Math.max(1, (termino - envio) / 1000));
                    }
                    concluidas.incrementAndGet();
                } catch (Exception e) {
                    erros.incrementAndGet();
                } finally {
                    emAndamento.decrementAndGet();
                }
            });
            indice++;
        }

        long fimDoDespacho = System.nanoTime();
        executor.shutdown();
        boolean drenou = executor.awaitTermination(60, TimeUnit.SECONDS);
        long cpuGasto = sistema.getProcessCpuTime() - cpuNoInicio;
        long relogioGasto = fimDoDespacho - relogioNoInicio;

        long medidas = latenciaCorrigida.getTotalCount();
        double taxaEfetiva = medidas > 0 ? medidas / (relogioGasto / 1_000_000_000.0) : 0;

        StringBuilder saida = new StringBuilder();
        saida.append("{\n");
        saida.append("  \"prototipo\": \"java-virtual-threads\",\n");
        saida.append("  \"taxa_alvo\": ").append(taxa).append(",\n");
        saida.append("  \"taxa_efetiva\": ").append(String.format("%.1f", taxaEfetiva)).append(",\n");
        saida.append("  \"enviadas\": ").append(enviadas.get()).append(",\n");
        saida.append("  \"concluidas\": ").append(concluidas.get()).append(",\n");
        saida.append("  \"erros\": ").append(erros.get()).append(",\n");
        saida.append("  \"medidas\": ").append(medidas).append(",\n");
        saida.append("  \"drenou\": ").append(drenou).append(",\n");
        saida.append("  \"pico_em_andamento\": ").append(picoEmAndamento.get()).append(",\n");
        saida.append("  \"despachos_atrasados\": ").append(despachosAtrasados.get()).append(",\n");
        saida.append("  \"cpu_ns_por_requisicao\": ").append(medidas > 0 ? cpuGasto / medidas : 0).append(",\n");
        saida.append("  \"cpu_percentual_de_um_nucleo\": ")
                .append(String.format("%.1f", relogioGasto > 0 ? 100.0 * cpuGasto / relogioGasto : 0)).append(",\n");
        saida.append(percentis("latencia_corrigida_us", latenciaCorrigida)).append(",\n");
        saida.append(percentis("latencia_de_servico_us", latenciaDeServico)).append(",\n");
        saida.append(percentis("desvio_de_agendamento_us", desvioDeAgendamento)).append("\n");
        saida.append("}");
        System.out.println(saida);
    }

    static String percentis(String nome, Histogram histograma) {
        return "  \"" + nome + "\": {"
                + "\"p50\": " + histograma.getValueAtPercentile(50)
                + ", \"p90\": " + histograma.getValueAtPercentile(90)
                + ", \"p99\": " + histograma.getValueAtPercentile(99)
                + ", \"p999\": " + histograma.getValueAtPercentile(99.9)
                + ", \"max\": " + histograma.getMaxValue()
                + ", \"amostras\": " + histograma.getTotalCount() + "}";
    }

    // Park sozinho erra na casa de milissegundos; a espera ativa final e o que
    // sustenta desvio de agendamento abaixo de 100 us em taxa alta.
    static void dormirAte(long instanteAlvo) {
        long restante = instanteAlvo - System.nanoTime();
        if (restante > 2_000_000L) {
            LockSupport.parkNanos(restante - 1_500_000L);
        }
        while (System.nanoTime() < instanteAlvo) {
            Thread.onSpinWait();
        }
    }

    static Map<String, String> lerOpcoes(String[] argumentos) {
        Map<String, String> opcoes = new HashMap<>();
        for (String argumento : argumentos) {
            String limpo = argumento.startsWith("-") ? argumento.substring(1) : argumento;
            int igual = limpo.indexOf('=');
            if (igual > 0) {
                opcoes.put(limpo.substring(0, igual), limpo.substring(igual + 1));
            }
        }
        return opcoes;
    }
}
