package app

import (
	"io"
	"rabbit/config"
	"rabbit/models"
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

var ErrClosed = errors.New("rmq closed")

type reconnecter struct {
	res chan error
}

type Rmq interface {
	io.Closer
	SendMessage(msg *models.Message) error
}

type rmq struct {
	wg  *sync.WaitGroup
	cfg *config.Config

	connection            *amqp.Connection
	channel               *amqp.Channel
	notifyChannelClose    chan *amqp.Error
	notifyConnectionClose chan *amqp.Error
	notifyConfirm         chan amqp.Confirmation
	err                   chan *reconnecter

	sendCh chan *models.MessageRequest

	close chan struct{}

	isConnected bool
}

func New(cfg *config.Config) (Rmq, error) {
	rmq := &rmq{
		wg:     &sync.WaitGroup{},
		cfg:    cfg,
		close:  make(chan struct{}),
		sendCh: make(chan *models.MessageRequest),
	}

	if err := rmq.Connect(); err != nil {
		return nil, errors.Wrap(err, "connect or rabbitmq error")
	}

	rmq.wg.Add(2)
	go rmq.reconnecter()
	go rmq.run()

	return rmq, nil
}

func (r *rmq) Connect() error {
	var err error

	r.connection, err = amqp.Dial(r.cfg.Addr)
	if err != nil {
		return errors.Wrap(err, `dial rabbit error`)
	}

	r.channel, err = r.connection.Channel()
	if err != nil {
		return errors.Wrap(err, `conn to channel error`)
	}

	r.connection.NotifyClose(r.notifyConnectionClose)
	r.channel.NotifyClose(r.notifyChannelClose)

	if err := r.channel.ExchangeDeclare(
		r.cfg.Exchange,      // name
		amqp.ExchangeDirect, // type
		true,                // durable
		false,               // auto-deleted
		false,               // internal
		false,               // noWait
		nil,                 // arguments
	); err != nil {
		return errors.Wrap(err, "declare exchange error")
	}
	
	r.isConnected = true

	return nil
}

func (r *rmq) listenCloseConn() {
	defer r.wg.Done()

main:
	for {
		select {
		case <-r.close:
			break main

		case <-r.notifyChannelClose:
			r.handleRec()

		case <-r.notifyConnectionClose:
			r.handleRec()
		}
	}
}

func (r *rmq) handleRec() error {
	res := make(chan error)
	defer close(res)

	for {
		r.err <- &reconnecter{
			res: res,
		}

		select {
		case <-r.close:
			return ErrClosed

		case err := <-res:
			if err != nil {
				continue
			} else {
				return nil
			}
		}
	}
}

func (r *rmq) reconnecter() {
	defer r.wg.Done()

main:
	for {
		select {
		case <-r.close:
			break main

		case rec := <-r.err:
			if err := r.Connect(); err != nil {
				rec.res <- errors.Wrap(err, "connect or rabbitmq error")
				continue
			}
			rec.res <- nil
		}
	}
}

// func (r *rmq) handleReconnect() {
// 	defer func() {
// 		r.wg.Done()
// 		logrus.Printf("Stop reconnecting to rabbitMQ")
// 	}()

// 	for r.alive {
// 		r.isConnected = false
// 		t := time.Now()
// 		fmt.Printf("Attempting to connect to rabbitMQ: %s\n", r.cfg.Addr)
// 		var retryCount int
// 		for !r.connect(r.cfg) {
// 			if !r.alive {
// 				return
// 			}
// 			select {
// 			case <-r.close:
// 				return
// 			case <-time.After(reconnectDelay + time.Duration(retryCount)*time.Second):
// 				logrus.Printf("disconnected from rabbitMQ and failed to connect")
// 				retryCount++
// 			}
// 		}
// 		logrus.Printf("Connected to rabbitMQ in: %vms", time.Since(t).Milliseconds())
// 		select {
// 		case <-r.close:
// 			return
// 		case <-r.notifyClose:
// 		}
// 	}
// }

// func (r *rmq) connect(cfg *config.Config) bool {
// 	conn, err := amqp.Dial(cfg.Addr)
// 	if err != nil {
// 		logrus.Errorf(`dial rabbit error: %v`, err)
// 		return false
// 	}

// 	ch, err := conn.Channel()
// 	if err != nil {
// 		logrus.Errorf(`conn to channel error: %v`, err)
// 		return false
// 	}

// 	exchange := `logs`
// 	if err := ch.ExchangeDeclare(
// 		exchange,
// 		amqp.ExchangeDirect,
// 		true,
// 		false,
// 		false,
// 		false,
// 		nil,
// 	); err != nil {
// 		logrus.Errorf(`declare exchange %s error: %v`, exchange, err)

// 		return false
// 	}

// 	r.changeConnection(conn, ch)
// 	r.isConnected = true

// 	return true
// }

// func (r *rmq) changeConnection(connection *amqp.Connection, channel *amqp.Channel) {
// 	r.connection = connection
// 	r.channel = channel
// 	r.notifyClose = make(chan *amqp.Error)
// 	r.notifyConfirm = make(chan amqp.Confirmation)
// 	r.channel.NotifyClose(r.notifyClose)
// 	r.channel.NotifyPublish(r.notifyConfirm)
// }

func (r *rmq) run() {
	defer r.wg.Done()

main:
	for {
		select {
		case <-r.close:
			break main

		case msg := <-r.sendCh:
			if !r.isConnected {
				msg.Err <- errors.New("rmq service is not avilable")
				continue
			}

			if err := r.channel.Publish(
				r.cfg.Exchange,
				r.cfg.RouteKey,
				false,
				false,
				msg.Message.PrepareToPublish(),
			); err != nil {
				msg.Err <- errors.Wrapf(err, "send message '%s' error", msg.Message.Text)
				continue
			}

			msg.Err <- nil

			logrus.Infof("message '%s' successfully sended", msg.Message.Text)
		}
	}

	logrus.Info("rmq sender closed!")
}

func (r *rmq) SendMessage(msg *models.Message) error {
	err := make(chan error)
	defer close(err)

	r.sendCh <- &models.MessageRequest{
		Message: msg,
		Err:     err,
	}

	return <-err
}

func (r *rmq) Close() error {
	select {
	case <-r.close:
		return ErrClosed
	default:
		close(r.close)
		r.wg.Wait()

		if r.channel != nil {
			if err := r.channel.Close(); err != nil {
				return err
			}
		}

		if r.connection != nil {
			if err := r.connection.Close(); err != nil {
				return err
			}
		}

		return nil
	}
}
