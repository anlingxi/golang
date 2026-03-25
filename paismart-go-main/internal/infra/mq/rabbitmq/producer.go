package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"pai-smart-go/internal/ai/history/event"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Config struct {
	URL             string
	Exchange        string
	ExchangeType    string
	RoutingKey      string
	Queue           string
	RetryQueue      string
	DeadLetterQueue string
	PrefetchCount   int
	ConsumerTag     string
}

type Producer struct {
	// 用的是amqp091-go库，连接和通道的类型是amqp.Connection和amqp.Channel
	conn *amqp.Connection
	ch   *amqp.Channel
	cfg  Config
}

func NewProducer(cfg Config) (*Producer, error) {
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, err
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	p := &Producer{
		conn: conn,
		ch:   ch,
		cfg:  cfg,
	}

	if err := p.declareTopology(); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}

	return p, nil
}

func (p *Producer) Close() error {
	if p.ch != nil {
		_ = p.ch.Close()
	}
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

// PublishPersistTurnTask 将 PersistTurnTask 任务发布到 RabbitMQ
func (p *Producer) PublishPersistTurnTask(ctx context.Context, task event.PersistTurnTask) error {
	body, err := json.Marshal(task)
	if err != nil {
		return err
	}

	return p.ch.PublishWithContext(
		ctx,
		p.cfg.Exchange,
		p.cfg.RoutingKey,
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
			MessageId:   task.TaskID,
		},
	)
}

// declareTopology 声明交换机、队列和绑定关系
func (p *Producer) declareTopology() error {
	if err := p.ch.ExchangeDeclare(
		p.cfg.Exchange,
		p.cfg.ExchangeType,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("declare exchange failed: %w", err)
	}

	_, err := p.ch.QueueDeclare(
		p.cfg.Queue,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("declare queue failed: %w", err)
	}

	if err := p.ch.QueueBind(
		p.cfg.Queue,
		p.cfg.RoutingKey,
		p.cfg.Exchange,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("bind queue failed: %w", err)
	}

	return nil
}
