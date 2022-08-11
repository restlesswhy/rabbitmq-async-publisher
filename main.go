package main

import (
	"os"
	"os/signal"

	"rabbit/app"
	"rabbit/config"

	"github.com/sirupsen/logrus"
)

func main() {
	cfg := config.Load()

	rmq := app.New(cfg)

	grace := make(chan os.Signal, 1)
	signal.Notify(grace, os.Interrupt)

	logrus.Info("Consumer started! To exit press Ctrl+C!")
	<-grace

	rmq.Close()

	logrus.Info("Gracefully closed!")
}
