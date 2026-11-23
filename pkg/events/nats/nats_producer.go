package nats_producer

import (
	producer_interfaces "github.com/evolution-foundation/evolution-go/pkg/events/interfaces"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	"github.com/gomessguii/logger"
	"github.com/nats-io/nats.go"
	"strings"
)

type natsProducer struct {
	conn              *nats.Conn
	natsGlobalEnabled bool
	natsGlobalEvents  []string
	loggerWrapper     *logger_wrapper.LoggerManager
}

func NewNatsProducer(
	url string,
	natsGlobalEnabled bool,
	natsGlobalEvents []string,
	loggerWrapper *logger_wrapper.LoggerManager,
) producer_interfaces.Producer {
	if strings.TrimSpace(url) == "" {
		return &natsProducer{
			conn:              nil,
			natsGlobalEnabled: false,
			natsGlobalEvents:  nil,
			loggerWrapper:     loggerWrapper,
		}
	}

	conn, err := nats.Connect(url)
	if err != nil {
		logger.LogError("Failed to connect to NATS: %v", err)
		return &natsProducer{
			conn:              nil,
			natsGlobalEnabled: false,
			natsGlobalEvents:  nil,
			loggerWrapper:     loggerWrapper,
		}
	}

	return &natsProducer{
		conn:              conn,
		natsGlobalEnabled: natsGlobalEnabled,
		natsGlobalEvents:  natsGlobalEvents,
		loggerWrapper:     loggerWrapper,
	}
}

func (p *natsProducer) Produce(
	queueName string,
	payload []byte,
	natsEnable string,
	userID string,
) error {
	p.loggerWrapper.GetLogger(userID).LogInfo("[%s] NATS Producer - Starting produce for subject: %s", userID, queueName)
	p.loggerWrapper.GetLogger(userID).LogInfo("[%s] NATS Producer - Global enabled: %v", userID, p.natsGlobalEnabled)

	if p.conn == nil {
		p.loggerWrapper.GetLogger(userID).LogWarn("[%s] NATS connection is nil", userID)
		return nil
	}

	if natsEnable == "global" {
		p.loggerWrapper.GetLogger(userID).LogInfo("[%s] Publishing to global subject: %s", userID, queueName)
		err := p.conn.Publish(queueName, payload)
		if err != nil {
			p.loggerWrapper.GetLogger(userID).LogError("[%s] Failed to publish message to subject %s: %v", userID, queueName, err)
			return err
		}
		p.loggerWrapper.GetLogger(userID).LogInfo("[%s] Message published successfully to subject: %s", userID, queueName)
	}

	if natsEnable == "enabled" {
		err := p.conn.Publish(queueName, payload)
		if err != nil {
			p.loggerWrapper.GetLogger(userID).LogError("[%s] Failed to publish message to instance subject %s: %v", userID, queueName, err)
			return err
		}
		p.loggerWrapper.GetLogger(userID).LogInfo("[%s] Message published successfully to instance subject: %s", userID, queueName)
	}

	return nil
}

// CreateGlobalQueues não faz nada para NATS producer pois os subjects são criados dinamicamente
func (p *natsProducer) CreateGlobalQueues() error {
	return nil
}
