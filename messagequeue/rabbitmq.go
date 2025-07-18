package messagequeue

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/config"
	"github.com/NHYCRaymond/go-backend-kit/monitoring"
	"github.com/rabbitmq/amqp091-go"
)

// RabbitMQ represents a RabbitMQ client
type RabbitMQ struct {
	config      config.RabbitMQConfig
	conn        *amqp091.Connection
	channels    map[string]*amqp091.Channel
	channelsMu  sync.RWMutex
	logger      *slog.Logger
	isConnected bool
	mu          sync.RWMutex
}

// NewRabbitMQ creates a new RabbitMQ client
func NewRabbitMQ(cfg config.RabbitMQConfig, logger *slog.Logger) (*RabbitMQ, error) {
	if logger == nil {
		logger = slog.Default()
	}

	rq := &RabbitMQ{
		config:   cfg,
		channels: make(map[string]*amqp091.Channel),
		logger:   logger,
	}

	if err := rq.connect(); err != nil {
		return nil, err
	}

	return rq, nil
}

// connect establishes connection to RabbitMQ
func (rq *RabbitMQ) connect() error {
	dsn := fmt.Sprintf("amqp://%s:%s@%s:%d/%s",
		rq.config.Username,
		rq.config.Password,
		rq.config.Host,
		rq.config.Port,
		rq.config.Vhost,
	)

	// Attempt to connect with retry logic
	for i := 0; i < 5; i++ {
		conn, err := amqp091.Dial(dsn)
		if err == nil {
			rq.mu.Lock()
			rq.conn = conn
			rq.isConnected = true
			rq.mu.Unlock()
			
			rq.logger.Info("RabbitMQ connected successfully")
			go rq.handleReconnect(dsn)
			return nil
		}
		
		rq.logger.Error("Failed to connect to RabbitMQ, retrying", "attempt", i+1, "error", err)
		time.Sleep(5 * time.Second)
	}

	return fmt.Errorf("failed to connect to RabbitMQ after 5 attempts")
}

// handleReconnect monitors connection and handles reconnection
func (rq *RabbitMQ) handleReconnect(dsn string) {
	for {
		rq.mu.RLock()
		conn := rq.conn
		rq.mu.RUnlock()

		if conn == nil {
			return
		}

		reason, ok := <-conn.NotifyClose(make(chan *amqp091.Error))
		if !ok {
			rq.logger.Warn("RabbitMQ connection close channel closed")
			return
		}

		rq.logger.Error("RabbitMQ connection closed, attempting to reconnect", "reason", reason)

		rq.mu.Lock()
		rq.isConnected = false
		rq.mu.Unlock()

		// Close all channels
		rq.channelsMu.Lock()
		for name, ch := range rq.channels {
			if ch != nil {
				ch.Close()
			}
			delete(rq.channels, name)
		}
		rq.channelsMu.Unlock()

		// Reconnect
		for {
			conn, err := amqp091.Dial(dsn)
			if err == nil {
				rq.mu.Lock()
				rq.conn = conn
				rq.isConnected = true
				rq.mu.Unlock()
				
				rq.logger.Info("RabbitMQ reconnected successfully")
				break
			}
			
			rq.logger.Error("Failed to reconnect to RabbitMQ, retrying", "error", err)
			time.Sleep(5 * time.Second)
		}
	}
}

// GetChannel returns a channel for the given name
func (rq *RabbitMQ) GetChannel(name string) (*amqp091.Channel, error) {
	rq.channelsMu.RLock()
	if ch, exists := rq.channels[name]; exists && ch != nil {
		rq.channelsMu.RUnlock()
		return ch, nil
	}
	rq.channelsMu.RUnlock()

	rq.mu.RLock()
	conn := rq.conn
	isConnected := rq.isConnected
	rq.mu.RUnlock()

	if !isConnected || conn == nil {
		return nil, fmt.Errorf("RabbitMQ connection not available")
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("failed to create channel: %w", err)
	}

	rq.channelsMu.Lock()
	rq.channels[name] = ch
	rq.channelsMu.Unlock()

	rq.logger.Info("RabbitMQ channel created", "name", name)
	return ch, nil
}

// Publish publishes a message to an exchange
func (rq *RabbitMQ) Publish(ctx context.Context, exchange, routingKey string, body []byte) error {
	start := time.Now()
	
	ch, err := rq.GetChannel("publisher")
	if err != nil {
		monitoring.RecordMessageQueueMetrics("rabbitmq", "publish", time.Since(start), err)
		return err
	}

	err = ch.PublishWithContext(ctx, exchange, routingKey, false, false, amqp091.Publishing{
		ContentType:  "application/json",
		Body:         body,
		Timestamp:    time.Now(),
		DeliveryMode: amqp091.Persistent,
	})

	monitoring.RecordMessageQueueMetrics("rabbitmq", "publish", time.Since(start), err)
	
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	return nil
}

// PublishWithOptions publishes a message with custom options
func (rq *RabbitMQ) PublishWithOptions(ctx context.Context, exchange, routingKey string, body []byte, options PublishOptions) error {
	start := time.Now()
	
	ch, err := rq.GetChannel("publisher")
	if err != nil {
		monitoring.RecordMessageQueueMetrics("rabbitmq", "publish", time.Since(start), err)
		return err
	}

	publishing := amqp091.Publishing{
		ContentType:  options.ContentType,
		Body:         body,
		Timestamp:    time.Now(),
		DeliveryMode: options.DeliveryMode,
		Priority:     options.Priority,
		Expiration:   options.Expiration,
		MessageId:    options.MessageID,
		UserId:       options.UserID,
		AppId:        options.AppID,
		Headers:      options.Headers,
	}

	err = ch.PublishWithContext(ctx, exchange, routingKey, options.Mandatory, options.Immediate, publishing)

	monitoring.RecordMessageQueueMetrics("rabbitmq", "publish", time.Since(start), err)
	
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	return nil
}

// Consume consumes messages from a queue
func (rq *RabbitMQ) Consume(ctx context.Context, queue string, handler MessageHandler) error {
	ch, err := rq.GetChannel(fmt.Sprintf("consumer_%s", queue))
	if err != nil {
		return err
	}

	msgs, err := ch.Consume(
		queue,
		"",    // consumer
		false, // auto-ack
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		return fmt.Errorf("failed to consume messages: %w", err)
	}

	rq.logger.Info("Started consuming messages", "queue", queue)

	go func() {
		for {
			select {
			case <-ctx.Done():
				rq.logger.Info("Stopping consumer", "queue", queue)
				return
			case msg, ok := <-msgs:
				if !ok {
					rq.logger.Warn("Message channel closed", "queue", queue)
					return
				}

				start := time.Now()
				err := handler(ctx, msg)
				
				monitoring.RecordMessageQueueMetrics(queue, "consume", time.Since(start), err)
				
				if err != nil {
					rq.logger.Error("Message processing failed", "queue", queue, "error", err)
					msg.Nack(false, true) // Requeue message
				} else {
					msg.Ack(false)
				}
			}
		}
	}()

	return nil
}

// DeclareExchange declares an exchange
func (rq *RabbitMQ) DeclareExchange(name, exchangeType string, options ExchangeOptions) error {
	ch, err := rq.GetChannel("setup")
	if err != nil {
		return err
	}

	return ch.ExchangeDeclare(
		name,
		exchangeType,
		options.Durable,
		options.AutoDelete,
		options.Internal,
		options.NoWait,
		options.Arguments,
	)
}

// DeclareQueue declares a queue
func (rq *RabbitMQ) DeclareQueue(name string, options QueueOptions) (amqp091.Queue, error) {
	ch, err := rq.GetChannel("setup")
	if err != nil {
		return amqp091.Queue{}, err
	}

	return ch.QueueDeclare(
		name,
		options.Durable,
		options.AutoDelete,
		options.Exclusive,
		options.NoWait,
		options.Arguments,
	)
}

// BindQueue binds a queue to an exchange
func (rq *RabbitMQ) BindQueue(queue, exchange, routingKey string, args amqp091.Table) error {
	ch, err := rq.GetChannel("setup")
	if err != nil {
		return err
	}

	return ch.QueueBind(queue, routingKey, exchange, false, args)
}

// Close closes the RabbitMQ connection
func (rq *RabbitMQ) Close() error {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	// Close all channels
	rq.channelsMu.Lock()
	for name, ch := range rq.channels {
		if ch != nil {
			ch.Close()
		}
		delete(rq.channels, name)
	}
	rq.channelsMu.Unlock()

	// Close connection
	if rq.conn != nil && !rq.conn.IsClosed() {
		err := rq.conn.Close()
		rq.isConnected = false
		rq.logger.Info("RabbitMQ connection closed")
		return err
	}

	return nil
}

// IsConnected returns the connection status
func (rq *RabbitMQ) IsConnected() bool {
	rq.mu.RLock()
	defer rq.mu.RUnlock()
	return rq.isConnected
}

// MessageHandler is a function that handles consumed messages
type MessageHandler func(ctx context.Context, msg amqp091.Delivery) error

// PublishOptions represents options for publishing messages
type PublishOptions struct {
	ContentType  string
	DeliveryMode uint8
	Priority     uint8
	Expiration   string
	MessageID    string
	UserID       string
	AppID        string
	Headers      amqp091.Table
	Mandatory    bool
	Immediate    bool
}

// ExchangeOptions represents options for declaring exchanges
type ExchangeOptions struct {
	Durable    bool
	AutoDelete bool
	Internal   bool
	NoWait     bool
	Arguments  amqp091.Table
}

// QueueOptions represents options for declaring queues
type QueueOptions struct {
	Durable    bool
	AutoDelete bool
	Exclusive  bool
	NoWait     bool
	Arguments  amqp091.Table
}

// DefaultPublishOptions returns default publishing options
func DefaultPublishOptions() PublishOptions {
	return PublishOptions{
		ContentType:  "application/json",
		DeliveryMode: amqp091.Persistent,
		Priority:     0,
		Mandatory:    false,
		Immediate:    false,
	}
}

// DefaultExchangeOptions returns default exchange options
func DefaultExchangeOptions() ExchangeOptions {
	return ExchangeOptions{
		Durable:    true,
		AutoDelete: false,
		Internal:   false,
		NoWait:     false,
		Arguments:  nil,
	}
}

// DefaultQueueOptions returns default queue options
func DefaultQueueOptions() QueueOptions {
	return QueueOptions{
		Durable:    true,
		AutoDelete: false,
		Exclusive:  false,
		NoWait:     false,
		Arguments:  nil,
	}
}