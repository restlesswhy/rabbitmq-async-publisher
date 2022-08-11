package main

import (
	"os"
	"os/signal"
	"time"

	"rabbit/app"
	"rabbit/config"

	"github.com/sirupsen/logrus"
)

func main() {
	cfg := config.Load()

	warnPublisher := app.New(cfg, &app.Params{
		Queue: "WARN",
		RouteKey: "warn_key",
		Delay: 1 * time.Second,
	})	

	// errorPublisher := app.New(cfg, &app.Params{
	// 	Queue: "ERROR",
	// 	RouteKey: "error_key",
	// 	Delay: 2 * time.Second,
	// })

	// debugPublisher := app.New(cfg, &app.Params{
	// 	Queue: "DEBUG",
	// 	RouteKey: "debug_key",
	// 	Delay: 3 * time.Second,
	// })

	grace := make(chan os.Signal, 1)
	signal.Notify(grace, os.Interrupt)

	logrus.Info("Consumer started! To exit press Ctrl+C!")
	<-grace

	warnPublisher.Close()
	// errorPublisher.Close()
	// debugPublisher.Close()
	logrus.Info("Gracefully closed!")
}
