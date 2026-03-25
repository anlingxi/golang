package rabbitmq

import (
	"context"
	"encoding/json"
	"pai-smart-go/internal/ai/history/event"
	"pai-smart-go/pkg/log"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Consumer struct {
	conn    *amqp.Connection
	ch      *amqp.Channel
	cfg     Config
	handler event.PersistTaskHandler
}

func NewConsumer(cfg Config, handler event.PersistTaskHandler) (*Consumer, error) {
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, err
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	c := &Consumer{
		conn:    conn,
		ch:      ch,
		cfg:     cfg,
		handler: handler,
	}

	if err := c.declareTopology(); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}

	if cfg.PrefetchCount > 0 {
		if err := ch.Qos(cfg.PrefetchCount, 0, false); err != nil {
			_ = ch.Close()
			_ = conn.Close()
			return nil, err
		}
	}

	return c, nil
}

func (c *Consumer) Close() error {
	if c.ch != nil {
		_ = c.ch.Close()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *Consumer) Start(ctx context.Context) error {
	msgs, err := c.ch.Consume(
		c.cfg.Queue,
		c.cfg.ConsumerTag,
		false, // manual ack
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-msgs:
			if !ok {
				return nil
			}
			c.handleMessage(ctx, msg)
		}
	}
}

func (c *Consumer) handleMessage(ctx context.Context, msg amqp.Delivery) {
	var task event.PersistTurnTask
	if err := json.Unmarshal(msg.Body, &task); err != nil {
		log.Errorf("[RabbitMQConsumer] unmarshal persist turn task failed: %v", err)
		_ = msg.Nack(false, false)
		return
	}

	if err := c.handler.HandlePersistTurnTask(ctx, task); err != nil {
		log.Errorf("[RabbitMQConsumer] handle persist turn task failed, task_id=%s, turn_id=%s, err=%v",
			task.TaskID, task.TurnID, err)
		_ = msg.Nack(false, true)
		return
	}

	_ = msg.Ack(false)
}

func (c *Consumer) declareTopology() error {
	// 先复用 producer 的同一套声明逻辑
	p := &Producer{ch: c.ch, cfg: c.cfg}
	return p.declareTopology()
}
