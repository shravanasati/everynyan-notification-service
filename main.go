package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"

	"github.com/Oudwins/zog"
	"github.com/SherClockHolmes/webpush-go"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

var errNoRequestBody = errors.New("missing request body")
var errInvalidJSON = errors.New("missing/invalid json in request body")

func readRequestBody(r *http.Request, w http.ResponseWriter) ([]byte, error) {
	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(errNoRequestBody.Error()))
		return nil, errNoRequestBody
	}

	return reqBody, nil
}

func getRequestBodyJSON(reqBody []byte, w http.ResponseWriter) (map[string]any, error) {
	reqMap := map[string]any{}
	err := json.Unmarshal(reqBody, &reqMap)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(errInvalidJSON.Error()))
		return nil, errInvalidJSON
	}

	return reqMap, nil
}

func failedZogValidation(errors map[string][]zog.ZogError, w http.ResponseWriter) {
	firstError := errors["$first"]
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte(firstError[0].Message()))
}

func main() {
	addr := "localhost:7924"
	storage := NewStorage("./subscriptions.db", "subscriptions")
	router := http.NewServeMux()
	connManager := NewWebsocketConnectionsManager()

	router.HandleFunc("/subscribe", func(w http.ResponseWriter, r *http.Request) {
		token, err := authorizeUserRequest(r, w)
		if err != nil {
			return
		}

		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			log.Println("unable to upgrade the http connection to websocket", err)
			// w.WriteHeader(http.StatusInternalServerError)
			// w.Write([]byte("unable to upgrade the http connection to websocket"))
			return
		}

		fmt.Println("new connection: ", conn.RemoteAddr())
		connManager.Add(token, conn)
		wsutil.WriteServerText(conn, []byte("notification subscription successfull"))
		// fmt.Println(authorConnMap)
		go func() {
			defer func(conn net.Conn, author string) {
				conn.Close()
				connManager.Delete(author)
			}(conn, token)

			for {
				msg, op, err := wsutil.ReadClientData(conn)
				var closedError wsutil.ClosedError
				if errors.As(err, &closedError) {
					fmt.Printf("%v broke connection \n", conn.RemoteAddr())
					break
				}
				if err != nil {
					fmt.Println("error in reading client data:", err)
					break
				}
				// handle ping-pong
				if string(msg) == "__ping__" {
					wsutil.WriteServerMessage(conn, op, []byte("__pong__"))
				}
				// fmt.Println(op, string(msg))
				// wsutil.WriteServerMessage(conn, op, msg)
			}
		}()
	})

	router.HandleFunc("POST /push-subscription", func(w http.ResponseWriter, r *http.Request) {
		reqBody, err := readRequestBody(r, w)
		if err != nil {
			return
		}

		reqMap, err := getRequestBodyJSON(reqBody, w)
		if err != nil {
			return
		}

		subscription := webpush.Subscription{}
		errors := pushSubscriptionSchema.Parse(reqMap, &subscription)
		if errors != nil {
			failedZogValidation(errors, w)
			return
		}

		token, err := authorizeUserRequest(r, w)
		if err != nil {
			return
		}

		err = storage.AddSubscription(token, subscription)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal server error, try again later"))
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("subscription added"))
	})

	router.HandleFunc("POST /send", func(w http.ResponseWriter, r *http.Request) {
		if err := authorizeAdminRequest(r, w); err != nil {
			return
		}

		reqBody, err := readRequestBody(r, w)
		if err != nil {
			return
		}

		reqMap, err := getRequestBodyJSON(reqBody, w)
		if err != nil {
			return
		}

		notifReqs := []NotificationRequest{}
		errors := notificationRequestSchema.Parse(reqMap, &notifReqs)
		if errors != nil {
			failedZogValidation(errors, w)
			return
		}

		log.Println("notification request accepted")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("notification request accepted"))

		// todo send to a worker pool
		for _, notif := range notifReqs {
			go func(notif NotificationRequest) {
				tokenConn, ok := connManager.Get(notif.User)
				if !ok {
					return
				}

				err = wsutil.WriteServerText(tokenConn, notif.TransmissionJSON())
				if err != nil {
					log.Println("error sending message to client:", err)
				}

				// push notifications
				sub, err := storage.GetSubscription(notif.User)
				if err != nil {
					log.Println("unable to get user subscription")
					return
				}
				sendPushNotification(PushNotificationEvent{
					Title: notif.Title,
					Body:  notif.Description,
					URL:   notif.Link,
					Icon:  "/android-192x192.png",
					Badge: "/logo.png",
					Image: "/logo.png",
				}, sub)
			}(notif)
		}
	})

	router.HandleFunc("POST /broadcast", func(w http.ResponseWriter, r *http.Request) {
		if err := authorizeAdminRequest(r, w); err != nil {
			return
		}

		reqBody, err := readRequestBody(r, w)
		if err != nil {
			return
		}

		reqMap, err := getRequestBodyJSON(reqBody, w)
		if err != nil {
			return
		}

		var broadcastRequest BroadcastRequest
		errors := broadcastRequestSchema.Parse(reqMap, &broadcastRequest)
		if errors != nil {
			failedZogValidation(errors, w)
			return
		}

		log.Println("broadcast request accepted")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("notification request accepted"))

		title := broadcastRequest.Title
		description := broadcastRequest.Description
		link := broadcastRequest.Link

		go func() {
			// websocket notifications
			wsMessage := NotificationRequest{
				Title:       title,
				Description: description,
				Link:        link,
			}.TransmissionJSON()

			for _, wsConn := range connManager.All() {
				go func(c net.Conn) {
					wsutil.WriteServerText(c, wsMessage)
				}(wsConn)
			}
		}()

		go func() {
			// push notifications
			pushMessage := jsonify(PushNotificationEvent{
				Title: title,
				Body:  description,
				URL:   link,
				Icon:  "/android-192x192.png",
				Badge: "/logo.png",
				Image: "/logo.png",
			})

			for sub := range storage.GetAllSubscriptions() {
				go _sendPushNotificationBytes(pushMessage, sub)
			}
		}()
	})

	log.Println("Ready to accept connections at", addr)
	err := http.ListenAndServe(addr, router)
	if err != nil {
		log.Fatalf("unable to start a server: %v \n", err)
	}

}
