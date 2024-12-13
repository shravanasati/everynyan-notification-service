package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

func getRequestBodyJSON[T any](reqBody []byte, w http.ResponseWriter) (T, error) {
	// reqMap := map[string]any{}
	var reqMap T
	err := json.Unmarshal(reqBody, &reqMap)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(errInvalidJSON.Error()))
		return reqMap, errInvalidJSON
	}

	return reqMap, nil
}

func failedZogValidation(errors map[string][]zog.ZogError, w http.ResponseWriter) {
	firstError := errors["$first"]
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte(firstError[0].Message()))
}

type wrappedWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *wrappedWriter) WriteHeader(statusCode int) {
	w.ResponseWriter.WriteHeader(statusCode)
	w.statusCode = statusCode
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/subscribe" {
			// dont include this route in middleware
			// because the wrapped one gives error
			// given ResponseWriter is not a http.Hijacker
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		wrapped := &wrappedWriter{
			ResponseWriter: w,
			statusCode: http.StatusOK,
		}

		next.ServeHTTP(wrapped, r)

		// eg. 200 GET /path in 150ms
		log.Printf("%v %v %v in %v\n", wrapped.statusCode, r.Method, r.URL.Path, time.Since(start))
	})
}

func main() {
	addr := "localhost:7924"
	storage := NewStorage("./subscriptions.db", "subscriptions")
	defer storage.db.Close()

	router := http.NewServeMux()
	connManager := NewWebsocketConnectionsManager()
	defer connManager.Close()

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

		connManager.Add(token, conn)

		go func() {
			defer func(conn net.Conn, author string) {
				conn.Close()
				connManager.Delete(author)
			}(conn, token)

			for {
				msg, op, err := wsutil.ReadClientData(conn)
				var closedError wsutil.ClosedError
				if errors.As(err, &closedError) {
					// fmt.Printf("%v broke connection \n", conn.RemoteAddr())
					break
				}
				if err != nil && err != io.EOF {
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

	router.HandleFunc("GET /connections", func(w http.ResponseWriter, r *http.Request) {
		if err := authorizeAdminRequest(r, w); err != nil {
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("%v", connManager.Count())))
	})

	router.HandleFunc("POST /push-subscription", func(w http.ResponseWriter, r *http.Request) {
		err := authorizeAdminRequest(r, w)
		if err != nil {
			return
		}

		reqBody, err := readRequestBody(r, w)
		if err != nil {
			return
		}

		reqMap, err := getRequestBodyJSON[map[string]any](reqBody, w)
		if err != nil {
			return
		}

		var subscription webpush.Subscription
		errors := pushSubscriptionSchema.Parse(reqMap, &subscription)
		if errors != nil {
			failedZogValidation(errors, w)
			return
		}

		token := r.URL.Query().Get("token")
		if token == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("missing token in url query"))
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

		reqMap, err := getRequestBodyJSON[[]map[string]any](reqBody, w)
		if err != nil {
			return
		}

		notifReqs := []NotificationRequest{}
		errors := notificationRequestSchema.Parse(reqMap, &notifReqs)
		if errors != nil {
			failedZogValidation(errors, w)
			return
		}

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

		reqMap, err := getRequestBodyJSON[map[string]any](reqBody, w)
		if err != nil {
			return
		}

		var broadcastRequest BroadcastRequest
		errors := broadcastRequestSchema.Parse(reqMap, &broadcastRequest)
		if errors != nil {
			failedZogValidation(errors, w)
			return
		}

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

	server := &http.Server{
		Addr:           addr,
		Handler:        loggingMiddleware(router),
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	// initializing the server in a goroutine so that
	// it wont block the graceful shutdown handling below
	go func() {
		log.Println("Ready to accept connections at", addr)
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("unable to start a server: %v \n", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server with
	// a timeout of 5 seconds.
	quit := make(chan os.Signal, 1)
	// kill (no param) default send syscall.SIGTERM
	// kill -2 is syscall.SIGINT
	// kill -9 is syscall.SIGKILL but can't be caught, so don't need to add it
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}
}
