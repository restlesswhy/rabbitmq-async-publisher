package main

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"time"

	"rabbit/app"
	"rabbit/config"
	"rabbit/models"

	"github.com/sirupsen/logrus"
)

const interval = 2

func main() {
	cfg := config.Load()
	wg := &sync.WaitGroup{}

	rmq, err := app.New(cfg)
	if err != nil {
		logrus.Fatal(err)
	}

	grace := make(chan os.Signal, 1)
	signal.Notify(grace, os.Interrupt)

	wg.Add(1)
	go func() {
		defer wg.Done()

		tick := time.NewTicker(2 * time.Second)
	
	sender:	
		for {
			select {
			case <-grace:
				break sender

			case <-tick.C:
				if err := rmq.SendMessage(&models.Message{
					Text: "asd",
				}); err != nil {
					logrus.Fatalf("send message error: %v", err)
				}
			}
		}
	
	logrus.Info("Stop sending messages")
	}()

	logrus.Info("Consumer started! To exit press Ctrl+C!")
	<-grace
	wg.Wait()
	fmt.Println()

	if err := rmq.Close(); err != nil {
		logrus.Warnf("Failed gracefully close rmq: %v", err)
	} else {
		logrus.Info("Gracefully closed!")
	}
}
