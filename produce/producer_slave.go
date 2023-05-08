// connection/channel handler to consume messages from monolith bus
package produce

import (
	"context"
	"errors"
	"fmt"
	"github.com/streadway/amqp"
	"log"
	"time"
)

// Channel handler
type Slave struct {
	master          *Master
	channel         *amqp.Channel
	done            chan struct{}
	notifyChanClose chan *amqp.Error
	notifyConfirm   chan amqp.Confirmation
	notifyFlow      chan bool
	IsReady         bool
	tm              *time.Ticker
}

var errConnNotReady = errors.New("Producer: connection not ready")

// creates a new consumer state instance, and automatically
// attempts to init to the channel.
func NewSlave(ctx context.Context, master *Master) *Slave {
	session := Slave{
		master: master,
		done:   make(chan struct{}),
		tm:     time.NewTicker(resendDelay),
	}
	go session.handleReInit(ctx)

	return &session
}

// will wait for a channel error
// and then continuously attempt to re-initialize both channels
func (s *Slave) handleReInit(ctx context.Context) bool {
	for {
		s.IsReady = false

		err := s.init(ctx)

		if err != nil {
			log.Println("Producer: failed to initialize channel (%s). Retrying...", err)

			select {
			case <-s.master.done:
				return true
			case <-s.done:
				return true
			case <-time.After(reInitDelay):
			}
			continue
		}

		select {
		case <-s.master.done:
			return true
		case <-s.done:
			return true
		case <-s.notifyChanClose:
			log.Println("Producer: channel closed. Re-init...")
		}
	}
}

// init will initialize channel & declare queue
func (s *Slave) init(ctx context.Context) error {
	for {
		if s.master.connection == nil || s.master.connection.IsClosed() {
			log.Println("Producer: connection not ready. Waiting...")
			time.Sleep(ReconnectDelay)
		} else {
			break
		}
	}

	ch, err := s.master.connection.Channel()
	if err != nil {
		return err
	}

	err = ch.Confirm(false)
	if err != nil {
		return err
	}

	err = s.declarationAndBinding(ctx, ch)
	if err != nil {
		return err
	}

	s.changeChannel(ctx, ch)
	s.IsReady = true
	s.done = make(chan struct{})
	log.Println("Producer: SETUP")

	return nil
}

// init queues and bindings
func (s *Slave) declarationAndBinding(_ context.Context, ch *amqp.Channel) (err error) {
	queues := []string{`q1`, `q2`, `q3`}
	queuesEntities := map[string][]string{
		`q1`: {`product`, `brand`},
		`q2`: {`category`},
		`q3`: {`product`, `attribute`},
	}

	for _, qName := range queues {
		_, err = ch.QueueDeclare(qName, true, false, false, false, nil)
		if err != nil {
			return
		}
	}

	for qName, entities := range queuesEntities {
		for _, entity := range entities {
			err = ch.QueueBind(qName, fmt.Sprintf("key-%s", entity), s.master.exName, false, nil)
			if err != nil {
				return
			}
		}
	}

	return
}

// takes a new channel to the queue,
// and updates the channel listeners to reflect this.
func (s *Slave) changeChannel(ctx context.Context, channel *amqp.Channel) {
	s.channel = channel
	s.notifyChanClose = make(chan *amqp.Error, 1)
	s.notifyConfirm = make(chan amqp.Confirmation, 1)
	s.channel.NotifyClose(s.notifyChanClose)
	s.channel.NotifyPublish(s.notifyConfirm)

	// research block, is this notification will be flashed
	s.notifyFlow = make(chan bool, 1)
	s.channel.NotifyFlow(s.notifyFlow)

	go s.listenFlow(ctx)
}
func (s *Slave) listenFlow(_ context.Context) {
	for {
		select {
		case res, ok := <-s.notifyFlow:
			log.Println("Producer: receive notifyFlow = %v, is closed = %v", res, ok)
			if !ok {
				return
			}
		}
	}
}

// will push data onto the queue, and wait for a confirm.
// If no confirms are received until within the resendTimeout,
// it continuously re-sends messages until a confirm is received.
// This will block until the server sends a confirm. Errors are
// only returned if the push action itself fails, see UnsafePush.
func (s *Slave) Push(_ context.Context, rk string, body []byte) error {
	tm := time.NewTicker(resendDelay)
	defer tm.Stop()

	retries := 0
	for {
		if !s.IsReady {
			if retries > pushRetries {
				return errors.New("Producer: failed to push")
			} else {
				log.Println("Producer: failed to push. Retrying...")
				retries++
				time.Sleep(ReconnectDelay)
			}
		} else {
			break
		}
	}

	retries = 0
	for {
		if !s.IsReady {
			return errConnNotReady
		}

		err := s.UnsafePush(rk, body)

		if err != nil {
			log.Println("Producer: Push failed: %s. (%s) Retrying...", err, rk)
			select {
			case <-s.master.done:
				log.Println("receive done signal from master %s", rk)
				return errShutdown
			case <-s.done:
				log.Println("receive done signal %s", rk)
				return errShutdown
			case <-tm.C:
			}
			continue
		}

		for {
			if !s.IsReady {
				return errConnNotReady
			}
			select {
			case confirm := <-s.notifyConfirm:
				if confirm.Ack {
					log.Println("Producer: published successfully into %s", rk)
					return nil
				} else {
					log.Println("producxer_slave, NOT Acked to %s", rk)
				}
			case <-s.master.done:
				log.Println("receive done signal from master to %s", rk)
				return nil
			case <-s.done:
				log.Println("receive done signal to %s", rk)
				return nil
			case <-tm.C:
				log.Println("producer_slave, relisten to %s", rk)
			}
			if s.master.connection.IsClosed() {
				return errConnNotReady
			}
			if retries > confirmRetries {
				return fmt.Errorf("Producer: failed to confirm to %s", rk)
			} else {
				retries++
				log.Println("Producer: failed to confirm. Retrying... to %s", rk)
			}
		}
	}
}

// will push to the queue without checking for
// confirmation. It returns an error if it fails to connect.
// No guarantees are provided for whether the server will
// recieve the message.
func (s *Slave) UnsafePush(rk string, body []byte) error {
	if !s.IsReady {
		return errors.New(fmt.Sprintf("Producer: connection not ready"))
	}

	return s.channel.Publish(
		s.master.exName,
		rk,
		false,
		false,
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/octet-stream",
			Body:         body,
			Priority:     5,
		},
	)
}

// will cleanly shutdown the channel and connection.
func (s *Slave) Close() error {
	if !s.IsReady {
		return errors.New(fmt.Sprintf("Producer: channel not ready while closing"))
	}
	err := s.channel.Close()
	if err != nil {
		return err
	}
	s.IsReady = false

	return nil
}

// ends all processes
func (s *Slave) Complete() {
	close(s.done)
}
