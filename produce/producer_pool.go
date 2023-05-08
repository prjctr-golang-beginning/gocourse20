// connection/channel handler to consume messages from monolith bus
package produce

import (
	"context"
	"log"
	"time"
)

// Master connection and slaves channels
type Pool struct {
	master *Master
	slaves []*Slave
}

// created new pool with one conn and N channels
func NewPool(ctx context.Context, threads int, exName, busHost, busUser, busPass string) *Pool {
	res := &Pool{master: NewMaster(ctx, exName, busHost, busUser, busPass)}

	for i := 0; i < threads; i++ {
		res.slaves = append(res.slaves, NewSlave(ctx, res.master))
	}

	return res
}

// all channels
func (s *Pool) Producers() []*Slave {
	return s.slaves
}

// close channels, then connection
func (s *Pool) Close(ctx context.Context) {
	log.Println("Producer: close slaves")
	for _, slave := range s.slaves {
		slave.Complete()
		err := slave.Close()
		if err != nil {
			log.Println(err)
		}
	}
	log.Println("Producer: slaves were closed, close master")
	s.master.Complete()
	time.Sleep(3 * time.Second)
	err := s.master.Close()
	if err != nil {
		log.Println(err)
	}
	log.Println("Producer: master closed")
}
