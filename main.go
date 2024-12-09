package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"

	z "github.com/Oudwins/zog"
	"github.com/SherClockHolmes/webpush-go"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

type NotificationRequest struct {
	User        string `json:"user,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Link        string `json:"link,omitempty"`
}

type BroadcastRequest struct {
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

var notificationRequestSchema = z.Slice(z.Struct(z.Schema{
	"user":        z.String().Required(z.Message("users array is required")).Min(1, z.Message("user cannot be empty")),
	"title":       z.String().Required(z.Message("title is required")).Min(1, z.Message("title cannot be empty")),
	"description": z.String().Required(z.Message("description is required")).Min(1, z.Message("description cannot be empty")),
	"link":        z.String().Required(z.Message("link is required")).Min(1, z.Message("link cannot be empty")),
}))

var broadcastRequestSchema = z.Struct(z.Schema{
	"title":       z.String().Required(z.Message("title is required")).Min(1, z.Message("title cannot be empty")),
	"description": z.String().Required(z.Message("description is required")).Min(1, z.Message("description cannot be empty")),
	"link":        z.String().Required(z.Message("link is required")).Min(1, z.Message("link cannot be empty")),
})

var pushSubscriptionSchema = z.Struct(z.Schema{
	"endpoint": z.String().Required(z.Message("endpoint URL is required")),
	"keys": z.Struct(z.Schema{
		"auth":   z.String().Required(z.Message("auth key is required")),
		"p256dh": z.String().Required(z.Message("p256dh key is required")),
	}),
})

type WebsocketConnectionsManager struct {
	authorConnMap map[string]net.Conn
	mu            sync.Mutex
}

func NewWebsocketConnectionsManager() *WebsocketConnectionsManager {
	return &WebsocketConnectionsManager{
		authorConnMap: make(map[string]net.Conn),
	}
}

func (manager *WebsocketConnectionsManager) Add(user string, conn net.Conn) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	manager.authorConnMap[user] = conn
}

func (manager *WebsocketConnectionsManager) Get(user string) (net.Conn, bool) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	conn, ok := manager.authorConnMap[user]
	return conn, ok
}

func (manager *WebsocketConnectionsManager) Delete(user string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	delete(manager.authorConnMap, user)
}

func (manager *WebsocketConnectionsManager) All() []net.Conn {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	return slices.Collect(maps.Values(manager.authorConnMap))
}

func authorizeUserRequest(r *http.Request, w http.ResponseWriter) (string, error) {
	sessionCookie, err := r.Cookie("session")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("missing cookies"))
		return "", err
	}

	success, token := checkAuth([]byte(sessionCookie.Value))
	if !success {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("unauthenticated"))
		return "", err
	}

	return token.Token, nil
}

func authorizeAdminRequest(r *http.Request, w http.ResponseWriter) error {
	auth := r.Header.Get("Authorization")
	splittedAuth := strings.Split(auth, " ")
	if len(splittedAuth) != 2 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("missing authorization"))
		return fmt.Errorf("missing auth")
	}

	apiKey := splittedAuth[1]
	if apiKey != API_KEY {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("invalid api key"))
		return fmt.Errorf("invalid api key")
	}

	return nil
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
		reqBody, err := io.ReadAll(r.Body)
		if err != nil {
			log.Println("unable to read request body:", err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("missing request body"))
			return
		}

		subscription := webpush.Subscription{}
		reqMap := map[string]any{}
		err = json.Unmarshal(reqBody, &reqMap)
		if err != nil {
			log.Println("unable to unmarshal request body:", err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("missing json in request body"))
			return
		}

		errors := pushSubscriptionSchema.Parse(reqMap, &subscription)
		if errors != nil {
			log.Println("zog validation failed")
			firstError := errors["$first"]
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(firstError[0].Message()))
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

		reqBody, err := io.ReadAll(r.Body)
		if err != nil {
			log.Println("unable to read request body:", err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("missing request body"))
			return
		}

		notifReqs := []NotificationRequest{}
		reqMap := []map[string]any{}
		err = json.Unmarshal(reqBody, &reqMap)
		if err != nil {
			log.Println("unable to unmarshal request body:", err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("missing json in request body"))
			return
		}

		errors := notificationRequestSchema.Parse(reqMap, &notifReqs)
		if errors != nil {
			log.Println("zog validation failed")
			firstError := errors["$first"]
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(firstError[0].Message()))
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

		reqBody, err := io.ReadAll(r.Body)
		if err != nil {
			log.Println("unable to read request body:", err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("missing request body"))
			return
		}

		var broadcastRequest BroadcastRequest
		reqMap := map[string]any{}
		err = json.Unmarshal(reqBody, &reqMap)
		if err != nil {
			log.Println("unable to unmarshal request body:", err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("missing json in request body"))
			return
		}

		errors := broadcastRequestSchema.Parse(reqMap, &broadcastRequest)
		if errors != nil {
			log.Println("zog validation failed")
			firstError := errors["$first"]
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(firstError[0].Message()))
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
