// Package kafka 提供了与 Kafka 消息队列交互的功能。
package kafka

import (
	"context"
	"encoding/json"
	"pai-smart-go/internal/config"
	"pai-smart-go/pkg/log"
	"pai-smart-go/pkg/tasks"
	"time"

	"github.com/segmentio/kafka-go"
)

const (
	maxProcessAttempts = 3
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
		ctx := context.Background()
		m, err := r.FetchMessage(ctx)
		if err != nil {
			log.Error("从 Kafka 读取消息失败", err)
			break // 退出循环，可能需要重启策略
		}

		log.Infof("收到 Kafka 消息: offset %d", m.Offset)

		var task tasks.FileProcessingTask
		if err := json.Unmarshal(m.Value, &task); err != nil {
			log.Errorf("无法解析 Kafka 消息: %v, value: %s", err, string(m.Value))
			// 消息格式错误，直接提交，避免阻塞队列
			if err := r.CommitMessages(ctx, m); err != nil {
				log.Errorf("提交错误消息失败: %v", err)
			}
			continue
		}

		log.Infof("开始处理文件任务: MD5=%s, FileName=%s", task.FileMD5, task.FileName)
		var processErr error
		for attempt := 1; attempt <= maxProcessAttempts; attempt++ {
			processErr = processor.Process(ctx, task)
			if processErr == nil {
				log.Infof("文件任务处理成功: MD5=%s, attempt=%d", task.FileMD5, attempt)
				if err := r.CommitMessages(ctx, m); err != nil {
					log.Errorf("提交 Kafka 消息 offset 失败: %v", err)
					processErr = err
					break
				}
				processErr = nil
				break
			}

			log.Errorf("处理文件任务失败: MD5=%s, attempt=%d/%d, Error: %v", task.FileMD5, attempt, maxProcessAttempts, processErr)
			if attempt < maxProcessAttempts {
				time.Sleep(time.Duration(attempt) * time.Second)
			}
		}

		if processErr != nil {
			log.Errorf("文件任务达到最大重试次数，停止消费以避免跳过失败消息: MD5=%s, Error: %v", task.FileMD5, processErr)
			break
		}
	}

	if err := r.Close(); err != nil {
		log.Fatalf("关闭 Kafka 消费者失败: %v", err)
	}
}
