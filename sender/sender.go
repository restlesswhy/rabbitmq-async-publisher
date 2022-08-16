package sender

import (
	"errors"
	"io"
	"rabbit/app"
	"rabbit/models"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

var ErrClosed = errors.New("rmq closed")

type sender struct {
	io.Closer

	rmq      app.Rmq
	close    chan struct{}
	wg       *sync.WaitGroup
	Interval int
}

func New(interval int, rmq app.Rmq) *sender {
	sender := &sender{
		wg:       &sync.WaitGroup{},
		Interval: interval,
		rmq:      rmq,
		close:    make(chan struct{}),
	}

	sender.wg.Add(1)
	go sender.run()

	return sender
}

func (s *sender) run() {
	defer s.wg.Done()

	t := time.NewTicker(time.Duration(s.Interval) * time.Second)

sender:
	for {
		select {
		case <-s.close:
			break sender

		case <-t.C:
			if err := s.rmq.SendMessage(&models.Message{
				Text: "error",
			}); err != nil {
				logrus.Errorf("send message error: %v", err)
			}
		}
	}

	logrus.Info("Sender is closed!")
}

func (s *sender) Close() error {
	select {
	case <-s.close:
		return ErrClosed
	default:
		logrus.Info("Closing sender...")
		close(s.close)
		s.wg.Wait()

		return nil
	}
}
