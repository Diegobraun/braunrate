package testsupport

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

var errNotLeader = errors.New("[6] Not Leader For Partition")

// The chain target starts by reading the offset of a topic it just created, and
// in that window the broker answers Not Leader For Partition. Waiting on the
// announced metadata was not enough: the broker publishes the leader before it
// answers as one, so the read has to be what retries.
func TestReadThatIsRefusedRightAfterCreationIsRetriedUntilTheBrokerSettles(t *testing.T) {
	refusals := 3
	attempts := 0

	err := untilReadyWithin(time.Second, time.Millisecond, "a particao 0", func() error {
		attempts++
		if attempts <= refusals {
			return errNotLeader
		}
		return nil
	})

	if err != nil {
		t.Fatalf("a leitura desistiu de um broker que ficou pronto: %v", err)
	}
	if attempts != refusals+1 {
		t.Fatalf("esperava %d tentativas, houve %d", refusals+1, attempts)
	}
}

// A broker that never settles is broken. Turning that into a slow start would
// hide it, so the wait is bounded and the last error goes up whole.
func TestBrokerThatNeverSettlesFailsWithTheReasonAndNotWithSilence(t *testing.T) {
	err := untilReadyWithin(30*time.Millisecond, time.Millisecond, "a particao 0 de \"pedidos\"", func() error {
		return errNotLeader
	})

	if err == nil {
		t.Fatal("um broker que nunca respondeu foi dado como pronto")
	}
	if !strings.Contains(err.Error(), "Not Leader For Partition") {
		t.Fatalf("a causa do broker se perdeu no caminho: %v", err)
	}
	if !strings.Contains(err.Error(), "a particao 0 de \"pedidos\"") {
		t.Fatalf("o erro nao diz o que nao ficou pronto: %v", err)
	}
}

// The unit test above proves the retry; only a real broker proves the window
// exists at all. Apache Kafka refused the read in two of three runs with topics
// created moments before, where Redpanda never did.
func TestProcessorStartsOnTopicsCreatedMomentsBefore(t *testing.T) {
	broker := os.Getenv("BRAUNRATE_KAFKA")
	if broker == "" {
		t.Skip("defina BRAUNRATE_KAFKA para rodar contra um broker real")
	}

	for round := 0; round < 3; round++ {
		t.Run(fmt.Sprintf("rodada-%d", round), func(t *testing.T) {
			stamp := time.Now().UnixNano()
			processor := NewProcessor(ProcessorOptions{
				Brokers: []string{broker},
				Input:   fmt.Sprintf("recem-criado-entrada-%d", stamp),
				Output:  fmt.Sprintf("recem-criado-saida-%d", stamp),
				Delay:   time.Millisecond,
			})
			if err := processor.Start(); err != nil {
				t.Fatalf("o processador nao subiu em topico recem-criado: %v", err)
			}
			_ = processor.Close()
		})
	}
}
