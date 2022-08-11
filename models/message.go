package models

import (
	"encoding/json"

	"github.com/streadway/amqp"
)

type MessageRequest struct {
	Message *Message
	Err     chan error
}

type Message struct {
	Text string `json:"text"`
}

func (m *Message) PrepareToPublish() amqp.Publishing {
	b, _ := json.Marshal(m)

	return amqp.Publishing{
		ContentType: "application/json",
		Body:        b,
	}
}
