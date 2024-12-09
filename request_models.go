package main

import (
	"encoding/json"
	"log"
)

type NotificationRequest struct {
	User        string `json:"user,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Link        string `json:"link,omitempty"`
}

func (notif NotificationRequest) TransmissionJSON() []byte {
	message, err := json.Marshal(map[string]string{
		"title":       notif.Title,
		"description": notif.Description,
		"link":        notif.Link,
	})
	if err != nil {
		log.Panic("unable to jsonify notifiaction request in transmissionJSON")
	}

	return message
}

type BroadcastRequest struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Link        string `json:"link,omitempty"`
}

