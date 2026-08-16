package kafka

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/Diegobraun/braunrate/internal/messaging"
	"github.com/Diegobraun/braunrate/internal/protocol"
	"github.com/segmentio/kafka-go"
)

const lagInterval = time.Second

// The time to produce says the broker accepted the message. Whether the service
// kept up is a different number, and it lives in the consumer group: lag is how
// many messages were written and not yet read. A run that produced in 1 ms and
// left the group ten thousand messages behind did not measure a healthy system.
type lagWatcher struct {
	group  string
	topic  string
	client *kafka.Client
	done   chan struct{}

	mutex sync.Mutex
	max   map[int]int64
	last  map[int]int64
	reads int
	fails string
}

func newLagWatcher(group, topic string, brokers []string, broker *messaging.Broker) (*lagWatcher, error) {
	transport, err := broker.Transport()
	if err != nil {
		return nil, err
	}
	client := &kafka.Client{Addr: kafka.TCP(brokers...), Timeout: 5 * time.Second}
	if transport != nil {
		client.Transport = transport
	}
	return &lagWatcher{
		group: group, topic: topic, client: client,
		max: map[int]int64{}, last: map[int]int64{},
		done: make(chan struct{}),
	}, nil
}

func (watcher *lagWatcher) watch(runContext context.Context) {
	defer close(watcher.done)
	ticker := time.NewTicker(lagInterval)
	defer ticker.Stop()
	for {
		watcher.sample(runContext)
		select {
		case <-ticker.C:
		case <-runContext.Done():
			// One last reading after the load stops: the interesting number is
			// how far behind the group was left, and that is only true at the end.
			watcher.sample(context.Background())
			return
		}
	}
}

// Lag is the difference between what was written and what the group committed.
// Both come from the broker, never from counting on this side: a message this
// generator did not send still counts against the service.
func (watcher *lagWatcher) sample(runContext context.Context) {
	partitions, err := watcher.client.Metadata(runContext, &kafka.MetadataRequest{Topics: []string{watcher.topic}})
	if err != nil || len(partitions.Topics) == 0 {
		watcher.note(err)
		return
	}

	numbers := make([]int, 0, len(partitions.Topics[0].Partitions))
	for _, partition := range partitions.Topics[0].Partitions {
		numbers = append(numbers, partition.ID)
	}

	ends, err := watcher.endOffsets(runContext, numbers)
	if err != nil {
		watcher.note(err)
		return
	}
	committed, err := watcher.committedOffsets(runContext, numbers)
	if err != nil {
		watcher.note(err)
		return
	}

	watcher.mutex.Lock()
	defer watcher.mutex.Unlock()
	watcher.reads++
	for number, end := range ends {
		position, has := committed[number]
		// A group that never committed on a partition has no lag to report:
		// zero here would claim it is up to date, which is the opposite.
		if !has || position < 0 {
			continue
		}
		lag := end - position
		if lag < 0 {
			lag = 0
		}
		watcher.last[number] = lag
		if lag > watcher.max[number] {
			watcher.max[number] = lag
		}
	}
}

func (watcher *lagWatcher) endOffsets(runContext context.Context, numbers []int) (map[int]int64, error) {
	request := &kafka.ListOffsetsRequest{Topics: map[string][]kafka.OffsetRequest{}}
	for _, number := range numbers {
		request.Topics[watcher.topic] = append(request.Topics[watcher.topic], kafka.LastOffsetOf(number))
	}
	response, err := watcher.client.ListOffsets(runContext, request)
	if err != nil {
		return nil, err
	}
	ends := map[int]int64{}
	for _, partition := range response.Topics[watcher.topic] {
		ends[partition.Partition] = partition.LastOffset
	}
	return ends, nil
}

func (watcher *lagWatcher) committedOffsets(runContext context.Context, numbers []int) (map[int]int64, error) {
	response, err := watcher.client.OffsetFetch(runContext, &kafka.OffsetFetchRequest{
		GroupID: watcher.group,
		Topics:  map[string][]int{watcher.topic: numbers},
	})
	if err != nil {
		return nil, err
	}
	committed := map[int]int64{}
	for _, partition := range response.Topics[watcher.topic] {
		committed[partition.Partition] = partition.CommittedOffset
	}
	return committed, nil
}

func (watcher *lagWatcher) note(err error) {
	if err == nil {
		return
	}
	watcher.mutex.Lock()
	watcher.fails = err.Error()
	watcher.mutex.Unlock()
}

func (watcher *lagWatcher) result() protocol.ConsumerLag {
	watcher.mutex.Lock()
	defer watcher.mutex.Unlock()

	lag := protocol.ConsumerLag{
		Group: watcher.group, Topic: watcher.topic,
		Readings: watcher.reads, Problem: watcher.fails,
		ByPartition: map[int]int64{},
	}
	numbers := make([]int, 0, len(watcher.max))
	for number := range watcher.max {
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)
	for _, number := range numbers {
		lag.ByPartition[number] = watcher.max[number]
		lag.Max += watcher.max[number]
		lag.Final += watcher.last[number]
	}
	return lag
}
