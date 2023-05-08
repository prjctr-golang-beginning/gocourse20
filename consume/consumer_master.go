// connection/channel handler to consume messages from monolith bus
package consume

import (
	"context"
	"errors"
	"fmt"
	"github.com/streadway/amqp"
	"log"
	"time"
)

// Master session handler
type Master struct {
	qName           string
	connection      *amqp.Connection
	done            chan struct{}
	notifyConnClose chan *amqp.Error
	isReady         bool
}

// NewSession creates a new consumer state instance, and automatically
// attempts to connect to the server.
func NewMaster(ctx context.Context, qName, busHost, busUser, busPass string) *Master {
	session := Master{
		qName: qName,
		done:  make(chan struct{}),
	}
	go session.handleReconnect(ctx, busHost, busUser, busPass)

	return &session
}

// handleReconnect will wait for a connection error on
// notifyConnClose, and then continuously attempt to reconnect.
func (s *Master) handleReconnect(ctx context.Context, busHost, busUser, busPass string) {
	for {
		s.isReady = false
		log.Println("Consumer: attempting to connect")

		err := s.connect(ctx, busHost, busUser, busPass)
		if err != nil {
			log.Println("Consumer: failed to connect (%s). Retrying...", err.Error())

			select {
			case <-s.done:
				return
			case <-time.After(ReconnectDelay):
			}
			continue
		}

		select {
		case <-s.done:
			return
		case <-s.notifyConnClose:
			log.Println("Consumer: connection closed. Reconnecting...")
		}
	}
}

// connect will create a new AMQP connection
func (s *Master) connect(ctx context.Context, busHost, busUser, busPass string) error {
	conn, err := amqp.DialConfig(busHost, amqp.Config{
		SASL: []amqp.Authentication{&amqp.PlainAuth{busUser, busPass}},
	})

	if err != nil {
		return err
	}

	s.changeConnection(conn)
	s.isReady = true
	log.Println("Consumer: CONNECTED")

	return nil
}

// changeConnection takes a new connection to the queue,
// and updates the close listener to reflect this.
func (s *Master) changeConnection(connection *amqp.Connection) {
	s.connection = connection
	s.notifyConnClose = make(chan *amqp.Error, 1)
	s.connection.NotifyClose(s.notifyConnClose)
}

// Close will cleanly shutdown the channel and connection.
func (s *Master) Close() error {
	if !s.isReady {
		return errors.New(fmt.Sprintf("Consumer: connection not ready while closing"))
	}
	err := s.connection.Close()
	if err != nil {
		return err
	}
	s.isReady = false

	return nil
}

func (s *Master) Complete() {
	s.done <- struct{}{}
}
