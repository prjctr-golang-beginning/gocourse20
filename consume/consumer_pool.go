// connection/channel handler to consume messages from monolith bus
package consume

import (
	"context"
	"log"
)

type Pool struct {
	master *Master
	slaves []*Slave
}

func NewPool(ctx context.Context, threads, prefetchCount int, qName, busHost, busUser, busPass string) *Pool {
	res := &Pool{master: NewMaster(ctx, qName, busHost, busUser, busPass)}

	for i := 0; i < threads; i++ {
		res.slaves = append(res.slaves, NewSlave(ctx, res.master, prefetchCount))
	}

	return res
}

func (s *Pool) Consumers() []*Slave {
	return s.slaves
}

func (s *Pool) Close(ctx context.Context) {
	for _, slave := range s.slaves {
		slave.Complete()
		err := slave.Close()
		if err != nil {
			log.Println(err)
		}
	}
	s.master.Complete()
	err := s.master.Close()
	if err != nil {
		log.Println(err)
	}
}
