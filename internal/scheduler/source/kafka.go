package source

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/perf-analysis/pkg/model"
	"github.com/perf-analysis/pkg/utils"
)

// SourceTypeKafka is the source type constant for Kafka source.
const SourceTypeKafka SourceType = "kafka"

func init() {
	// Register the Kafka source strategy
	Register(SourceTypeKafka, NewKafkaSource)
}

// KafkaOptions holds Kafka source specific configuration.
type KafkaOptions struct {
	// Brokers is the list of Kafka broker addresses.
	Brokers []string

	// Topic is the Kafka topic to consume from.
	Topic string

	// ConsumerGroup is the consumer group ID.
	ConsumerGroup string

	// AutoCommit enables automatic offset commit.
	AutoCommit bool

	// MaxPollRecords is the maximum number of records to poll at once.
	MaxPollRecords int
}

// DefaultKafkaOptions returns the default options.
func DefaultKafkaOptions() *KafkaOptions {
	return &KafkaOptions{
		Brokers:        []string{"localhost:9092"},
		Topic:          "perf-tasks",
		ConsumerGroup:  "perf-analyzer",
		AutoCommit:     false,
		MaxPollRecords: 100,
	}
}

// KafkaMessage represents a message from Kafka containing task data.
type KafkaMessage struct {
	Task   *model.Task `json:"task"`
	Offset int64       `json:"-"` // Kafka offset for acknowledgment
}

// KafkaSource implements TaskSource for Kafka-based task consumption.
type KafkaSource struct {
	name    string
	options *KafkaOptions
	logger  utils.Logger

	taskChan chan *TaskEvent
	stopCh   chan struct{}

	mu      sync.RWMutex
	running bool

	// consumer would be the actual Kafka consumer (e.g., sarama, confluent-kafka-go)
	// consumer kafka.Consumer
}

// NewKafkaSource creates a new Kafka source from configuration.
func NewKafkaSource(cfg *SourceConfig) (TaskSource, error) {
	opts := &KafkaOptions{
		Brokers:        cfg.GetStringSlice("brokers", []string{"localhost:9092"}),
		Topic:          cfg.GetString("topic", "perf-tasks"),
		ConsumerGroup:  cfg.GetString("consumer_group", "perf-analyzer"),
		AutoCommit:     cfg.GetBool("auto_commit", false),
		MaxPollRecords: cfg.GetInt("max_poll_records", 100),
	}

	return &KafkaSource{
		name:     cfg.Name,
		options:  opts,
		taskChan: make(chan *TaskEvent, opts.MaxPollRecords),
		stopCh:   make(chan struct{}),
	}, nil
}

// NewKafkaSourceWithOptions creates a new Kafka source with explicit options.
func NewKafkaSourceWithOptions(name string, opts *KafkaOptions, logger utils.Logger) *KafkaSource {
	if opts == nil {
		opts = DefaultKafkaOptions()
	}
	if logger == nil {
		logger = utils.NewDefaultLogger(utils.LevelInfo, nil)
	}

	return &KafkaSource{
		name:     name,
		options:  opts,
		logger:   logger,
		taskChan: make(chan *TaskEvent, opts.MaxPollRecords),
		stopCh:   make(chan struct{}),
	}
}

// SetLogger sets the logger.
func (s *KafkaSource) SetLogger(logger utils.Logger) {
	s.logger = logger
}

// Type returns the source type.
func (s *KafkaSource) Type() SourceType {
	return SourceTypeKafka
}

// Name returns the source instance name.
func (s *KafkaSource) Name() string {
	return s.name
}

// Start starts the Kafka consumer.
func (s *KafkaSource) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true
	s.mu.Unlock()

	if s.logger != nil {
		s.logger.Info("Kafka source %s starting with brokers=%v, topic=%s, group=%s",
			s.name, s.options.Brokers, s.options.Topic, s.options.ConsumerGroup)
	}

	// TODO: Initialize actual Kafka consumer here
	// Example with sarama:
	// config := sarama.NewConfig()
	// config.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRoundRobin
	// consumer, err := sarama.NewConsumerGroup(s.options.Brokers, s.options.ConsumerGroup, config)
	// if err != nil {
	//     return err
	// }
	// s.consumer = consumer

	go s.consumeLoop(ctx)
	return nil
}

// Stop stops the Kafka consumer.
func (s *KafkaSource) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	s.mu.Unlock()

	close(s.stopCh)

	// TODO: Close actual Kafka consumer
	// if s.consumer != nil {
	//     return s.consumer.Close()
	// }

	return nil
}

// Tasks returns the task event channel.
func (s *KafkaSource) Tasks() <-chan *TaskEvent {
	return s.taskChan
}

// Ack acknowledges a task has been processed successfully.
// For Kafka source, this commits the message offset.
func (s *KafkaSource) Ack(ctx context.Context, event *TaskEvent) error {
	// TODO: Commit Kafka offset
	// offset := event.AckToken.(int64)
	// return s.consumer.CommitOffset(s.options.Topic, partition, offset)

	if s.logger != nil {
		s.logger.Debug("Kafka source %s acked task %s", s.name, event.ID)
	}
	return nil
}

// Nack indicates a task processing failed.
// For Kafka source, this could send to a dead letter queue or retry topic.
func (s *KafkaSource) Nack(ctx context.Context, event *TaskEvent, reason string) error {
	// TODO: Send to dead letter queue or retry topic
	// dlqTopic := s.options.Topic + ".dlq"
	// return s.producer.Send(dlqTopic, event.Task)

	if s.logger != nil {
		s.logger.Warn("Kafka source %s nacked task %s: %s", s.name, event.ID, reason)
	}
	return nil
}

// HealthCheck checks the Kafka connection.
func (s *KafkaSource) HealthCheck(ctx context.Context) error {
	// TODO: Check Kafka broker connectivity
	// return s.consumer.Ping()
	return nil
}

// consumeLoop continuously consumes messages from Kafka.
func (s *KafkaSource) consumeLoop(ctx context.Context) {
	// TODO: Implement actual Kafka consumption
	// This is a placeholder showing the expected flow

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		default:
			// TODO: Poll messages from Kafka
			// messages, err := s.consumer.Poll(100)
			// if err != nil {
			//     s.logger.Error("Kafka poll error: %v", err)
			//     continue
			// }
			//
			// for _, msg := range messages {
			//     task, err := s.parseMessage(msg)
			//     if err != nil {
			//         s.logger.Error("Failed to parse Kafka message: %v", err)
			//         continue
			//     }
			//
			//     event := NewTaskEvent(task, SourceTypeKafka, s.name).
			//         WithAckToken(msg.Offset).
			//         WithMetadata("partition", strconv.Itoa(int(msg.Partition))).
			//         WithMetadata("offset", strconv.FormatInt(msg.Offset, 10))
			//
			//     select {
			//     case s.taskChan <- event:
			//     case <-ctx.Done():
			//         return
			//     case <-s.stopCh:
			//         return
			//     }
			// }

			// Placeholder: wait for stop signal
			select {
			case <-ctx.Done():
				return
			case <-s.stopCh:
				return
			}
		}
	}
}

// parseMessage parses a Kafka message into a Task.
func (s *KafkaSource) parseMessage(data []byte) (*model.Task, error) {
	var msg KafkaMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return msg.Task, nil
}
