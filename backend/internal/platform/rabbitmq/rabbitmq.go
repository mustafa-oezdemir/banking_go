// Package rabbitmq provides the Banking outbox publisher and Notification
// consumer adapters. It uses durable exchanges, queues, and persistent messages.
package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog/log"

	"github.com/mustafa-oezdemir/banking_go/internal/notification"
	"github.com/mustafa-oezdemir/banking_go/internal/platform/notificationstore"
)

const (
	bankingExchange    = "banking.events"
	retryExchange      = "banking.events.retry"
	deadLetterExchange = "banking.events.dlx"
	notificationQueue  = "notification.payment.v1"
	retryQueue         = "notification.payment.retry"
	deadLetterQueue    = "notification.payment.dlq"
)

// Config is shared by the publisher and consumer. URL uses amqp:// or amqps://.
type Config struct {
	URL            string
	DialTimeout    time.Duration
	RetryDelay     time.Duration
	MaxRetries     int
	DeliveryWindow time.Duration
}

// ConfigFromEnvironment loads RabbitMQ configuration without logging secrets.
func ConfigFromEnvironment() (Config, error) {
	config := Config{
		URL: strings.TrimSpace(os.Getenv("RABBITMQ_URL")), DialTimeout: 5 * time.Second,
		RetryDelay: 5 * time.Second, MaxRetries: 3, DeliveryWindow: 12 * time.Second,
	}
	if raw := strings.TrimSpace(os.Getenv("RABBITMQ_RETRY_DELAY")); raw != "" {
		value, err := time.ParseDuration(raw)
		if err != nil || value <= 0 {
			return Config{}, errors.New("RABBITMQ_RETRY_DELAY must be a positive duration")
		}
		config.RetryDelay = value
	}
	if raw := strings.TrimSpace(os.Getenv("RABBITMQ_MAX_RETRIES")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 || value > 20 {
			return Config{}, errors.New("RABBITMQ_MAX_RETRIES must be between 0 and 20")
		}
		config.MaxRetries = value
	}
	if raw := strings.TrimSpace(os.Getenv("NOTIFICATION_DELIVERY_TIMEOUT")); raw != "" {
		value, err := time.ParseDuration(raw)
		if err != nil || value <= 0 {
			return Config{}, errors.New("NOTIFICATION_DELIVERY_TIMEOUT must be a positive duration")
		}
		config.DeliveryWindow = value
	}
	return config, config.Validate()
}

// Validate checks local configuration before a background loop starts.
func (config Config) Validate() error {
	parsed, err := url.Parse(config.URL)
	if err != nil || (parsed.Scheme != "amqp" && parsed.Scheme != "amqps") || parsed.Host == "" {
		return errors.New("RABBITMQ_URL must be an absolute amqp(s) URL")
	}
	if config.DialTimeout <= 0 || config.RetryDelay <= 0 || config.MaxRetries < 0 || config.DeliveryWindow <= 0 {
		return errors.New("invalid RabbitMQ timeout or retry configuration")
	}
	return nil
}

func (config Config) dial() (*amqp.Connection, error) {
	dialer := net.Dialer{Timeout: config.DialTimeout}
	return amqp.DialConfig(config.URL, amqp.Config{
		Dial: func(network, address string) (net.Conn, error) {
			return dialer.Dial(network, address)
		},
	})
}

func declareTopology(channel *amqp.Channel, retryDelay time.Duration) error {
	if err := channel.ExchangeDeclare(bankingExchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}
	if err := channel.ExchangeDeclare(retryExchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}
	if err := channel.ExchangeDeclare(deadLetterExchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := channel.QueueDeclare(notificationQueue, true, false, false, false, nil); err != nil {
		return err
	}
	if err := channel.QueueBind(notificationQueue, "payment.*.v1", bankingExchange, false, nil); err != nil {
		return err
	}
	retryArgs := amqp.Table{
		"x-message-ttl":          int32(retryDelay.Milliseconds()),
		"x-dead-letter-exchange": bankingExchange,
	}
	if _, err := channel.QueueDeclare(retryQueue, true, false, false, false, retryArgs); err != nil {
		return err
	}
	if err := channel.QueueBind(retryQueue, "payment.*.v1", retryExchange, false, nil); err != nil {
		return err
	}
	if _, err := channel.QueueDeclare(deadLetterQueue, true, false, false, false, nil); err != nil {
		return err
	}
	return channel.QueueBind(deadLetterQueue, "notification.payment.dead", deadLetterExchange, false, nil)
}

// Publisher sends persistent event envelopes after an outbox row is committed.
type Publisher struct{ config Config }

// NewPublisher constructs a validated publisher.
func NewPublisher(config Config) (*Publisher, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &Publisher{config: config}, nil
}

// Enabled reports whether this publisher has usable RabbitMQ configuration.
func (publisher *Publisher) Enabled() bool {
	return publisher != nil && publisher.config.Validate() == nil
}

// Publish uses broker publisher confirms. If confirmation cannot be observed,
// the outbox row remains unpublished and will be delivered again later.
func (publisher *Publisher) Publish(ctx context.Context, event notification.EventEnvelope) error {
	if err := event.Validate(); err != nil {
		return err
	}
	connection, err := publisher.config.dial()
	if err != nil {
		return fmt.Errorf("connect RabbitMQ: %w", err)
	}
	defer connection.Close()
	channel, err := connection.Channel()
	if err != nil {
		return fmt.Errorf("open RabbitMQ channel: %w", err)
	}
	defer channel.Close()
	if err = declareTopology(channel, publisher.config.RetryDelay); err != nil {
		return fmt.Errorf("declare RabbitMQ topology: %w", err)
	}
	if err = channel.Confirm(false); err != nil {
		return fmt.Errorf("enable RabbitMQ confirmations: %w", err)
	}
	confirmations := channel.NotifyPublish(make(chan amqp.Confirmation, 1))
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode RabbitMQ event: %w", err)
	}
	if err = channel.PublishWithContext(ctx, bankingExchange, event.EventType, false, false, amqp.Publishing{
		ContentType: "application/json", DeliveryMode: amqp.Persistent, MessageId: event.EventID.String(),
		Timestamp: event.OccurredAt, CorrelationId: event.CorrelationID, Type: event.EventType, Body: body,
	}); err != nil {
		return fmt.Errorf("publish RabbitMQ event: %w", err)
	}
	select {
	case confirmation, open := <-confirmations:
		if !open || !confirmation.Ack {
			return errors.New("RabbitMQ did not confirm event publication")
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// EventStore is Notification's persistence port for idempotent processing.
type EventStore interface {
	Claim(context.Context, uuid.UUID) (notificationstore.ClaimState, error)
	MarkProcessed(context.Context, uuid.UUID) error
	Release(context.Context, uuid.UUID, string) error
}

// ActivityDelivery is the provider-independent Notification delivery port.
type ActivityDelivery interface {
	DeliverActivity(context.Context, notification.ActivityCommand) error
}

// Outcome tells the AMQP adapter whether to acknowledge, retry, or dead-letter.
type Outcome string

const (
	Processed Outcome = "processed"
	Duplicate Outcome = "duplicate"
	Retry     Outcome = "retry"
	Dead      Outcome = "dead"
)

// Processor handles the idempotent part of consumption without RabbitMQ I/O.
type Processor struct {
	store    EventStore
	delivery ActivityDelivery
	timeout  time.Duration
}

// NewProcessor constructs a testable idempotent event processor.
func NewProcessor(store EventStore, delivery ActivityDelivery, timeout time.Duration) (*Processor, error) {
	if store == nil || delivery == nil {
		return nil, errors.New("notification event store and delivery are required")
	}
	if timeout <= 0 {
		return nil, errors.New("notification delivery timeout is required")
	}
	return &Processor{store: store, delivery: delivery, timeout: timeout}, nil
}

// Process validates one event, claims it, delivers it, and stores completion.
func (processor *Processor) Process(ctx context.Context, event notification.EventEnvelope) Outcome {
	if err := event.Validate(); err != nil {
		return Dead
	}
	claim, err := processor.store.Claim(ctx, event.EventID)
	if err != nil {
		return Retry
	}
	switch claim {
	case notificationstore.AlreadyProcessed:
		return Duplicate
	case notificationstore.Busy:
		return Retry
	case notificationstore.Claimed:
	default:
		return Retry
	}
	command, err := event.ActivityCommand()
	if err != nil {
		_ = processor.store.Release(ctx, event.EventID, "invalid event payload")
		return Dead
	}
	deliveryCtx, cancel := context.WithTimeout(ctx, processor.timeout)
	err = processor.delivery.DeliverActivity(deliveryCtx, command)
	cancel()
	if err != nil {
		_ = processor.store.Release(ctx, event.EventID, "delivery failed")
		return Retry
	}
	if err = processor.store.MarkProcessed(ctx, event.EventID); err != nil {
		return Retry
	}
	return Processed
}

// Consumer connects to RabbitMQ and applies Processor outcomes with manual ack.
type Consumer struct {
	config    Config
	processor *Processor
}

// NewConsumer creates a durable Notification consumer.
func NewConsumer(config Config, processor *Processor) (*Consumer, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if processor == nil {
		return nil, errors.New("notification processor is required")
	}
	return &Consumer{config: config, processor: processor}, nil
}

// Run reconnects after transient broker failures until context cancellation.
func (consumer *Consumer) Run(ctx context.Context) {
	for {
		if err := consumer.consumeOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Warn().Err(err).Msg("Notification RabbitMQ consumer disconnected")
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(consumer.config.RetryDelay):
		}
	}
}

func (consumer *Consumer) consumeOnce(ctx context.Context) error {
	connection, err := consumer.config.dial()
	if err != nil {
		return err
	}
	defer connection.Close()
	channel, err := connection.Channel()
	if err != nil {
		return err
	}
	defer channel.Close()
	if err = declareTopology(channel, consumer.config.RetryDelay); err != nil {
		return err
	}
	if err = channel.Qos(10, 0, false); err != nil {
		return err
	}
	deliveries, err := channel.Consume(notificationQueue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case delivery, open := <-deliveries:
			if !open {
				return errors.New("RabbitMQ delivery channel closed")
			}
			consumer.handleDelivery(ctx, channel, delivery)
		}
	}
}

func (consumer *Consumer) handleDelivery(ctx context.Context, channel *amqp.Channel, delivery amqp.Delivery) {
	var event notification.EventEnvelope
	if err := json.Unmarshal(delivery.Body, &event); err != nil {
		consumer.deadLetter(ctx, channel, delivery, "invalid JSON event")
		return
	}
	outcome := consumer.processor.Process(ctx, event)
	switch outcome {
	case Processed, Duplicate:
		if err := delivery.Ack(false); err != nil {
			log.Warn().Err(err).Str("event_id", event.EventID.String()).Msg("Notification ack failed")
		}
	case Retry:
		if retryCount(delivery.Headers) >= consumer.config.MaxRetries {
			consumer.deadLetter(ctx, channel, delivery, "retry limit reached")
			return
		}
		if err := consumer.publishCopy(ctx, channel, retryExchange, delivery.RoutingKey, delivery.Body, delivery.Headers, retryHeaders(delivery.Headers)); err != nil {
			log.Warn().Err(err).Str("event_id", event.EventID.String()).Msg("Notification retry publish failed")
			_ = delivery.Nack(false, true)
			return
		}
		if err := delivery.Ack(false); err != nil {
			log.Warn().Err(err).Str("event_id", event.EventID.String()).Msg("Notification retry ack failed")
		}
	case Dead:
		consumer.deadLetter(ctx, channel, delivery, "invalid or unsupported event")
	}
}

func (consumer *Consumer) deadLetter(ctx context.Context, channel *amqp.Channel, delivery amqp.Delivery, reason string) {
	if err := consumer.publishCopy(ctx, channel, deadLetterExchange, "notification.payment.dead", delivery.Body, delivery.Headers,
		amqp.Table{"x-dead-letter-reason": reason}); err != nil {
		log.Warn().Err(err).Msg("Notification dead-letter publish failed")
		_ = delivery.Nack(false, true)
		return
	}
	if err := delivery.Ack(false); err != nil {
		log.Warn().Err(err).Msg("Notification dead-letter ack failed")
	}
}

func (consumer *Consumer) publishCopy(ctx context.Context, channel *amqp.Channel, exchange, routingKey string, body []byte, headers, extra amqp.Table) error {
	merged := amqp.Table{}
	for key, value := range headers {
		merged[key] = value
	}
	for key, value := range extra {
		merged[key] = value
	}
	return channel.PublishWithContext(ctx, exchange, routingKey, false, false, amqp.Publishing{
		ContentType: "application/json", DeliveryMode: amqp.Persistent, Headers: merged, Body: body,
	})
}

func retryCount(headers amqp.Table) int {
	value, ok := headers["x-retry-count"]
	if !ok {
		return 0
	}
	switch number := value.(type) {
	case int32:
		return int(number)
	case int64:
		return int(number)
	case int:
		return number
	default:
		return 0
	}
}

func retryHeaders(headers amqp.Table) amqp.Table {
	return amqp.Table{"x-retry-count": int32(retryCount(headers) + 1)}
}
