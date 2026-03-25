// Package kafka 提供了与 Kafka 消息队列交互的功能。
package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"pai-smart-go/internal/config"
	"pai-smart-go/pkg/database"
	"pai-smart-go/pkg/log"
	"pai-smart-go/pkg/tasks"
	"time"

	"github.com/segmentio/kafka-go"
)

// TaskProcessor defines the interface for any service that can process a task.
// This decouples the Kafka consumer from the concrete pipeline implementation.
type TaskProcessor interface {
	Process(ctx context.Context, task tasks.FileProcessingTask) error
}

var producer *kafka.Writer

// InitProducer 初始化 Kafka 生产者。
func InitProducer(cfg config.KafkaConfig) {
	producer = &kafka.Writer{
		Addr:     kafka.TCP(cfg.Brokers),
		Topic:    cfg.Topic,
		Balancer: &kafka.LeastBytes{},
	}
	log.Info("Kafka 生产者初始化成功")
}

// ProduceFileTask 发送一个文件处理任务到 Kafka。
func ProduceFileTask(task tasks.FileProcessingTask) error {
	taskBytes, err := json.Marshal(task)
	if err != nil {
		return err
	}
	// 这里传入一个ctx，但是好像没人控制这个ctx的生命周期，暂时用Background，后续可以考虑增加一个全局的ctx或者传入一个可控的ctx
	// 这里的kafka只传入了value，没有key，意味着所有消息会被随机分配到不同的分区，如果需要保证同一文件的任务在同一分区，
	// 可以考虑使用fileMD5作为key
	err = producer.WriteMessages(context.Background(),
		kafka.Message{
			Value: taskBytes,
			Key:   []byte(task.FileMD5),
		},
	)
	return err
}

// StartConsumer 启动一个 Kafka 消费者来处理文件任务。
// 这里有因为consumer处理的慢被kafka踢出的设计嘛？没有，kafka的消费者组机制会自动处理消费者的负载均衡和故障转移，
// 如果一个消费者处理消息过慢或者发生故障，Kafka 会将它从消费者组中移除，并将它负责的分区重新分配给其他消费者。
// 这种机制确保了消息能够被及时处理，同时也提高了系统的可靠性和可伸缩性。
func StartConsumer(cfg config.KafkaConfig, processor TaskProcessor) {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{cfg.Brokers},
		Topic:    cfg.Topic,
		GroupID:  "pai-smart-go-consumer",
		MinBytes: 10e3, // 10KB
		MaxBytes: 10e6, // 10MB
	})

	log.Infof("Kafka 消费者已启动，正在监听主题 '%s'", cfg.Topic)

	for {
		m, err := r.FetchMessage(context.Background())
		if err != nil {
			log.Error("从 Kafka 读取消息失败", err)
			break // 退出循环，可能需要重启策略
		}

		log.Infof("收到 Kafka 消息: offset %d", m.Offset)

		var task tasks.FileProcessingTask
		if err := json.Unmarshal(m.Value, &task); err != nil {
			log.Errorf("无法解析 Kafka 消息: %v, value: %s", err, string(m.Value))
			// 消息格式错误，直接提交，避免阻塞队列
			if err := r.CommitMessages(context.Background(), m); err != nil {
				log.Errorf("提交错误消息失败: %v", err)
			}
			continue
		}

		log.Infof("开始处理文件任务: MD5=%s, FileName=%s", task.FileMD5, task.FileName)
		// 同步处理任务
		// 这里利用了redis的计数功能来实现简单的失败重试机制，避免某个消息因为处理失败而一直阻塞在队列中
		if err := processor.Process(context.Background(), task); err != nil {
			log.Errorf("处理文件任务失败: MD5=%s, Error: %v", task.FileMD5, err)
			// 使用 Redis 计数失败次数，达到阈值后提交 offset 终止重试
			attemptsKey := fmt.Sprintf("kafka:attempts:%s", task.FileMD5)
			attempts, incErr := database.RDB.Incr(context.Background(), attemptsKey).Result()
			if incErr == nil {
				_ = database.RDB.Expire(context.Background(), attemptsKey, 24*time.Hour).Err()
			}
			if incErr != nil {
				// Redis 异常时保守处理：不提交 offset，让 Kafka 重试
				continue
			}
			// 这里提交后，消息不久丢了嘛？是的，提交后消息就丢了，所以这里的逻辑是：当同一文件的任务失败次数达到3次时，认为这个任务有问题，
			// 不再重试，直接提交 offset 丢弃这个消息。对于真正偶尔失败的任务，最多会重试3次，增加成功的机会；对于一直失败的任务，
			// 最多也只会重试3次，避免一直阻塞队列。
			if attempts >= 3 {
				log.Errorf("文件任务多次失败(>=3)，提交 offset 终止重试: MD5=%s", task.FileMD5)
				if err := r.CommitMessages(context.Background(), m); err != nil {
					log.Errorf("提交 Kafka 消息 offset 失败: %v", err)
				}
			}
			// attempts < 3 时，不提交 offset 让 Kafka 自动重试
		} else {
			log.Infof("文件任务处理成功: MD5=%s", task.FileMD5)
			// 清理失败计数
			_ = database.RDB.Del(context.Background(), fmt.Sprintf("kafka:attempts:%s", task.FileMD5)).Err()
			// 任务处理成功后，手动提交 offset
			if err := r.CommitMessages(context.Background(), m); err != nil {
				log.Errorf("提交 Kafka 消息 offset 失败: %v", err)
			}
		}
	}

	if err := r.Close(); err != nil {
		log.Fatalf("关闭 Kafka 消费者失败: %v", err)
	}
}
// Package kafka 提供了与 Kafka 消息队列交互的功能。
package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"pai-smart-go/internal/config"
	"pai-smart-go/pkg/database"
	"pai-smart-go/pkg/log"
	"pai-smart-go/pkg/tasks"
	"time"

	"github.com/segmentio/kafka-go"
)

// TaskProcessor defines the interface for any service that can process a task.
// This decouples the Kafka consumer from the concrete pipeline implementation.
type TaskProcessor interface {
	Process(ctx context.Context, task tasks.FileProcessingTask) error
}

var producer *kafka.Writer

// InitProducer 初始化 Kafka 生产者。
func InitProducer(cfg config.KafkaConfig) {
	producer = &kafka.Writer{
		Addr:     kafka.TCP(cfg.Brokers),
		Topic:    cfg.Topic,
		Balancer: &kafka.LeastBytes{},
	}
	log.Info("Kafka 生产者初始化成功")
}

// ProduceFileTask 发送一个文件处理任务到 Kafka。
func ProduceFileTask(task tasks.FileProcessingTask) error {
	taskBytes, err := json.Marshal(task)
	if err != nil {
		return err
	}

	err = producer.WriteMessages(context.Background(),
		kafka.Message{
			Value: taskBytes,
		},
	)
	return err
}

// StartConsumer 启动一个 Kafka 消费者来处理文件任务。
func StartConsumer(cfg config.KafkaConfig, processor TaskProcessor) {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{cfg.Brokers},
		Topic:    cfg.Topic,
		GroupID:  "pai-smart-go-consumer",
		MinBytes: 10e3, // 10KB
		MaxBytes: 10e6, // 10MB
	})

	log.Infof("Kafka 消费者已启动，正在监听主题 '%s'", cfg.Topic)

	for {
		m, err := r.FetchMessage(context.Background())
		if err != nil {
			log.Error("从 Kafka 读取消息失败", err)
			break // 退出循环，可能需要重启策略
		}

		log.Infof("收到 Kafka 消息: offset %d", m.Offset)

		var task tasks.FileProcessingTask
		if err := json.Unmarshal(m.Value, &task); err != nil {
			log.Errorf("无法解析 Kafka 消息: %v, value: %s", err, string(m.Value))
			// 消息格式错误，直接提交，避免阻塞队列
			if err := r.CommitMessages(context.Background(), m); err != nil {
				log.Errorf("提交错误消息失败: %v", err)
			}
			continue
		}

		log.Infof("开始处理文件任务: MD5=%s, FileName=%s", task.FileMD5, task.FileName)
		// 同步处理任务
		if err := processor.Process(context.Background(), task); err != nil {
			log.Errorf("处理文件任务失败: MD5=%s, Error: %v", task.FileMD5, err)
			// 使用 Redis 计数失败次数，达到阈值后提交 offset 终止重试
			attemptsKey := fmt.Sprintf("kafka:attempts:%s", task.FileMD5)
			attempts, incErr := database.RDB.Incr(context.Background(), attemptsKey).Result()
			if incErr == nil {
				_ = database.RDB.Expire(context.Background(), attemptsKey, 24*time.Hour).Err()
			}
			if incErr != nil {
				// Redis 异常时保守处理：不提交 offset，让 Kafka 重试
				continue
			}
			if attempts >= 3 {
				log.Errorf("文件任务多次失败(>=3)，提交 offset 终止重试: MD5=%s", task.FileMD5)
				if err := r.CommitMessages(context.Background(), m); err != nil {
					log.Errorf("提交 Kafka 消息 offset 失败: %v", err)
				}
			}
			// attempts < 3 时，不提交 offset 让 Kafka 自动重试
		} else {
			log.Infof("文件任务处理成功: MD5=%s", task.FileMD5)
			// 清理失败计数
			_ = database.RDB.Del(context.Background(), fmt.Sprintf("kafka:attempts:%s", task.FileMD5)).Err()
			// 任务处理成功后，手动提交 offset
			if err := r.CommitMessages(context.Background(), m); err != nil {
				log.Errorf("提交 Kafka 消息 offset 失败: %v", err)
			}
		}
	}

	if err := r.Close(); err != nil {
		log.Fatalf("关闭 Kafka 消费者失败: %v", err)
	}
}
