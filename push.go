package main

import (
	"fmt"
	"log"
	"os"

	"github.com/SherClockHolmes/webpush-go"
	_ "github.com/joho/godotenv/autoload"
)

var vapidPublicKey string
var vapidPrivateKey string

func init() {
	var found bool
	vapidPublicKey, found = os.LookupEnv("VAPID_PUBLIC_KEY")
	if !found {
		fmt.Println(webpush.GenerateVAPIDKeys())
		panic("VAPID PUBLIC KEY NOT SET")
	}
	vapidPrivateKey, found = os.LookupEnv("VAPID_PRIVATE_KEY")
	if !found {
		fmt.Println(webpush.GenerateVAPIDKeys())
		panic("VAPID PRIVATE KEY NOT SET")
	}
}

type PushNotificationEvent struct {
	Body  string `json:"body"`
	Icon  string `json:"icon"`
	Image string `json:"image"`
	Badge string `json:"badge"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

func sendPushNotification(notif PushNotificationEvent, subscription webpush.Subscription) {
	_sendPushNotificationBytes(jsonify(notif), subscription)
}

func _sendPushNotificationBytes(message []byte, subscription webpush.Subscription) {
	fmt.Println("sending this push notification:", string(message))
	resp, err := webpush.SendNotification(message, &subscription, &webpush.Options{
		Subscriber: "dev.shravan@proton.me",
		VAPIDPublicKey: vapidPublicKey,
		VAPIDPrivateKey: vapidPrivateKey,
		Urgency: webpush.UrgencyNormal,
	})

	if err != nil {
		log.Println("unable to send push notification", err)
		return
	}

	defer resp.Body.Close()
}