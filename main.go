package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"

	z "github.com/Oudwins/zog"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

type NotificationRequest struct {
	User        string `json:"user,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Link        string `json:"link,omitempty"`
}

var notificationRequestSchema = z.Slice(z.Struct(z.Schema{
	"users":       z.String().Required(z.Message("users array is required")).Min(1, z.Message("user cannot be empty")),
	"title":       z.String().Required(z.Message("title is required")).Min(1, z.Message("title cannot be empty")),
	"description": z.String().Required(z.Message("description is required")).Min(1, z.Message("description cannot be empty")),
	"link":        z.String().Required(z.Message("link is required")).Min(1, z.Message("link cannot be empty")),
}))

var authorConnMap = make(map[string]net.Conn)

func main() {
	addr := "localhost:7924"
	router := http.NewServeMux()
	router.HandleFunc("/subscribe", func(w http.ResponseWriter, r *http.Request) {
		sessionCookie, err := r.Cookie("session")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("missing cookies"))
			return
		}

		success, token := checkAuth([]byte(sessionCookie.Value))
		if !success {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("unauthenticated"))
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
		authorConnMap[token] = conn
		wsutil.WriteServerText(conn, []byte("notification subscription successfull"))
		// fmt.Println(authorConnMap)
		go func() {
			defer func (conn net.Conn, author string)  {
				conn.Close()
				delete(authorConnMap, author)
			} (conn, token)

			for {
				msg, op, err := wsutil.ReadClientData(conn)
				var closedError wsutil.ClosedError
				if errors.As(err, &closedError) {
					fmt.Printf("%v broke connection", conn.RemoteAddr())
					break
				}
				if err != nil {
					fmt.Println("error in reading client data:", err)
					break
				}
				// handle ping-pong
				if string(msg) == "__ping__"{
					wsutil.WriteServerMessage(conn, op, []byte("__pong__"))
				}
				// fmt.Println(op, string(msg))
				// wsutil.WriteServerMessage(conn, op, msg)
			}
		}()
	})

	router.HandleFunc("POST /send", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		splittedAuth := strings.Split(auth, " ")
		if len(splittedAuth) != 2 {
			log.Println("missing auth")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("missing authorization"))
			return
		}

		apiKey := splittedAuth[1]
		if apiKey != API_KEY {
			log.Println("invalid api key")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("invalid api key"))
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

		fmt.Println("notification request accepted")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("notification request accepted"))

		// todo send to a worker pool
		for _, notif := range notifReqs {
			go func(notif NotificationRequest) {
				userConn, ok := authorConnMap[notif.User]
				if !ok {
					return
				}

				message, err := json.Marshal(map[string]string{
					"title":       notif.Title,
					"description": notif.Description,
					"link":        notif.Link,
				})
				if err != nil {
					log.Panic("unable to jsonify")
				}
				err = wsutil.WriteServerText(userConn, message)
				if err != nil {
					log.Println("error sending message to client:", err)
				}
			}(notif)
		}
	})

	log.Println("Ready to accept connections at", addr)
	err := http.ListenAndServe(addr, router)
	if err != nil {
		log.Fatalf("unable to start a server: %v \n", err)
	}

}
