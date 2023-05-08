package main

import (
	"fmt"
	"time"
)

func producer(ch chan<- int) {
	for i := 0; i < 5; i++ {
		fmt.Println("Sending", i)
		ch <- i
	}
}

func consumer(ch <-chan int) {
	for i := range ch {
		fmt.Println("Received", i)
		time.Sleep(1 * time.Second)
	}
}

func main() {
	ch := make(chan int)

	go producer(ch)
	go consumer(ch)

	time.Sleep(6 * time.Second)
}
