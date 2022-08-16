package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"

	"rabbit/app"
	"rabbit/config"
	"rabbit/sender"

	"github.com/sirupsen/logrus"
)

const interval = 3

func main() {
	cfg := config.Load()
	wg := &sync.WaitGroup{}
	closers := make([]io.Closer, 0, 2)

	rmq, err := app.New(cfg)
	if err != nil {
		logrus.Fatal(err)
	}
	
	sender := sender.New(interval, rmq)

	closers = append(closers, rmq, sender)

	grace := make(chan os.Signal, 1)
	signal.Notify(grace, os.Interrupt)

	logrus.Info("Consumer started! To exit press Ctrl+C!")
	<-grace
	wg.Wait()
	fmt.Println()
	logrus.Info("Closing rmq...")

	for i := range closers {
		if err := closers[i].Close(); err != nil {
			logrus.Warnf("Failed gracefully close rmq: %v", err)
		} else {
			logrus.Info("Gracefully closed!")
		}
	}
}
