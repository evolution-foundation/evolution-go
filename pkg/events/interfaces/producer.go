package producer_interfaces

type Producer interface {
	Produce(queueName string, payload []byte, webhookUrl string, userID string) error
	CreateGlobalQueues() error
}

// why: kept separate from Producer so adding exchange support to RabbitMQ
// doesn't force nats/webhook/websocket producers to implement it too.
type ExchangePublisher interface {
	ProduceToExchange(exchange string, routingKey string, payload []byte, userID string) error
}
