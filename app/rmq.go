package app

import (
	"encoding/json"
	"fmt"
	"io"
	"rabbit/config"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
)

const (
	// When reconnecting to the server after connection failure
	reconnectDelay = 5 * time.Second

	// When resending messages the server didn't confirm
	resendDelay = 5 * time.Second
)

var ErrClosed = errors.New("rbt closed")

type Params struct {
	Queue    string
	RouteKey string
	Delay    time.Duration
}

type Message struct {
	Text string `json:"text"`
}

type Rmq interface {
	io.Closer
}

type rmq struct {
	wg  *sync.WaitGroup
	cfg *config.Config

	connection    *amqp.Connection
	channel       *amqp.Channel
	notifyClose   chan *amqp.Error
	notifyConfirm chan amqp.Confirmation

	close chan struct{}

	delay    time.Duration
	queue    string
	routeKey string

	isConnected bool
	alive       bool
}

func New(cfg *config.Config, params *Params) Rmq {
	rmq := &rmq{
		wg:       &sync.WaitGroup{},
		cfg:      cfg,
		close:    make(chan struct{}),
		delay:    params.Delay,
		queue:    params.Queue,
		routeKey: params.RouteKey,
		alive:    true,
	}

	rmq.wg.Add(2)
	go rmq.handleReconnect()
	go rmq.run()

	return rmq
}

func (r *rmq) handleReconnect() {
	defer func() {
		r.wg.Done()
		logrus.Printf("Stop reconnecting to rabbitMQ")
	}()

	for r.alive {
		r.isConnected = false
		t := time.Now()
		fmt.Printf("Attempting to connect to rabbitMQ: %s\n", r.cfg.Addr)
		var retryCount int
		for !r.connect(r.cfg) {
			if !r.alive {
				return
			}
			select {
			case <-r.close:
				return
			case <-time.After(reconnectDelay + time.Duration(retryCount)*time.Second):
				logrus.Printf("disconnected from rabbitMQ and failed to connect")
				retryCount++
			}
		}
		logrus.Printf("Connected to rabbitMQ in: %vms", time.Since(t).Milliseconds())
		select {
		case <-r.close:
			return
		case <-r.notifyClose:
		}
	}
}

func (r *rmq) connect(cfg *config.Config) bool {
	conn, err := amqp.Dial(cfg.Addr)
	if err != nil {
		logrus.Errorf(`dial rabbit error: %v`, err)
		return false
	}

	ch, err := conn.Channel()
	if err != nil {
		logrus.Errorf(`conn to channel error: %v`, err)
		return false
	}

	_, err = ch.QueueDeclare(
		r.queue,
		true,  // Durable
		false, // Delete when unused
		false, // Exclusive
		false, // No-wait
		nil,   // Arguments
	)
	if err != nil {
		logrus.Errorf(`declare %s queue error: %v`, r.queue, err)
		return false
	}

	exchange := fmt.Sprintf(`direct_%s`, r.queue)
	if err := ch.ExchangeDeclare(
		exchange,
		amqp.ExchangeDirect,
		true,
		true,
		false,
		false,
		nil,
	); err != nil {
		logrus.Errorf(`declare exchange %s error: %v`, exchange, err)

		return false
	}

	if err := ch.QueueBind(r.queue, r.routeKey, exchange, false, nil); err != nil {
		logrus.Errorf(`bina queue %s error: %v`, r.queue, err)

		return false
	}

	r.changeConnection(conn, ch)
	r.isConnected = true

	return true
}

func (r *rmq) changeConnection(connection *amqp.Connection, channel *amqp.Channel) {
	r.connection = connection
	r.channel = channel
	r.notifyClose = make(chan *amqp.Error)
	r.notifyConfirm = make(chan amqp.Confirmation)
	r.channel.NotifyClose(r.notifyClose)
	r.channel.NotifyPublish(r.notifyConfirm)
}

func (r *rmq) run() {
	defer r.wg.Done()

	t := time.Tick(r.delay)

main:
	for {
		select {
		case <-r.close:
			break main

		case <-t:
			r.channel.Publish(
				fmt.Sprintf(`direct_%s`, r.queue),
				r.routeKey,
				false,
				false,
				amqp.Publishing{
					ContentType: "text/plain",
					Body:        []byte(r.queue),
				},
			)
		case msg, ok := <-r.recvQ:
			if !ok {
				// connectionDropped = true
				return
			}
			var res Message
			if err := json.Unmarshal(msg.Body, &res); err != nil {
				logrus.Errorf("unmarshal body error: %v", err)
				break
			}
			fmt.Printf("MSG: %v\n", res)
		}
	}

	logrus.Info("RBT closed!")
}

func (r *rmq) Close() error {
	select {
	case <-r.close:
		return ErrClosed
	default:
		close(r.close)
		r.wg.Wait()

		err := r.channel.Close()
		if err != nil {
			return err
		}

		err = r.connection.Close()
		if err != nil {
			return err
		}

		return nil
	}
}
