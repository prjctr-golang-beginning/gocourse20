// connection/channel handler to consume messages from monolith bus
package consume

import (
	"errors"
	"time"
)

type MonolithMessage interface {
	UnmarshalJSON(data []byte) error
}

const (
	// When reconnecting to the server after connection failure
	ReconnectDelay = 5 * time.Second
	// When setting up the channel after a channel exception
	reInitDelay = 2 * time.Second
)

var (
	errNotConnected = errors.New("Consumer: not connected to the server")
)
