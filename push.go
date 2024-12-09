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
	Body  string `json:"body,omitempty"`
	Icon  string `json:"icon,omitempty"`
	Image string `json:"image,omitempty"`
	Badge string `json:"badge,omitempty"`
	URL   string `json:"url,omitempty"`
	Title string `json:"title,omitempty"`
}

func sendPushNotification(notif PushNotificationEvent, subscription webpush.Subscription) {
	resp, err := webpush.SendNotification(jsonify(notif), &subscription, &webpush.Options{
		Subscriber: "dev.shravan@proton.me",
		VAPIDPublicKey: vapidPublicKey,
		VAPIDPrivateKey: vapidPrivateKey,
	})

	if err != nil {
		log.Println("unable to send notification")
		return
	}

	defer resp.Body.Close()
}
