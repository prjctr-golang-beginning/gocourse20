// connection/channel handler to push messages in "store style" to GoodsService bus
package produce

import (
	"errors"
	"time"
)

const (
	// When reconnecting to the server after connection failure
	ReconnectDelay = 5 * time.Second
	// When setting up the channel after a channel exception
	reInitDelay = 5 * time.Second
	// When resending messages the server didn't confirm
	resendDelay    = 15 * time.Second
	confirmRetries = 9
	pushRetries    = 3
)

var errShutdown = errors.New("-- Producer session shut down")
