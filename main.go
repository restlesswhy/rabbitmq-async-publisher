package main

import (
	"os"
	"os/signal"

	"rabbit/app"
	"rabbit/config"
	"rabbit/models"

	"github.com/sirupsen/logrus"
)

func main() {
	cfg := config.Load()

	rmq, err := app.New(cfg)
	if err != nil {
		logrus.Fatal(err)
	}

	if err := rmq.SendMessage(&models.Message{
		Text: "asd",
	}); err != nil {
		logrus.Fatalf("send message error: %v", err)
	}

	grace := make(chan os.Signal, 1)
	signal.Notify(grace, os.Interrupt)

	logrus.Info("Consumer started! To exit press Ctrl+C!")
	<-grace

	rmq.Close()

	logrus.Info("Gracefully closed!")
}
