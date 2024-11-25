package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"runtime"

	"github.com/gobwas/httphead"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

var authorMap = make(map[string]net.Conn)

func main() {
	addr := "localhost:7924"
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to start a tcp server at `%v`, err: %v \n", addr, err)
	}

	// Prepare handshake header writer from http.Header mapping.
	header := ws.HandshakeHeaderHTTP(http.Header{
		"X-Go-Version": []string{runtime.Version()},
	})

	u := ws.Upgrader{
		// OnHost: func(host []byte) error {
		// 	fmt.Println("host:", string(host))
		// 	if string(host) == "github.com" {
		// 		return nil
		// 	}
		// 	return ws.RejectConnectionError(
		// 		ws.RejectionStatus(403),
		// 		ws.RejectionHeader(ws.HandshakeHeaderString(
		// 			"X-Want-Host: github.com\r\n",
		// 		)),
		// 	)
		// },
		OnHeader: func(key, value []byte) error {
			// fmt.Printf("header key=%v value=%v\n", string(key), string(value))
			if string(key) != "Cookie" {
				// fmt.Println("skipping because key not cookie")
				return nil
			}
			// todo check for authentication
			ok := httphead.ScanCookie(value, func(key, value []byte) bool {
				fmt.Println(string(key), string(value))
				return checkAuth(value)
			})
			if ok {
				return nil
			}
			return ws.RejectConnectionError(
				ws.RejectionReason("bad cookie"),
				ws.RejectionStatus(400),
			)
		},
		OnBeforeUpgrade: func() (ws.HandshakeHeader, error) {
			return header, nil
		},
	}

	log.Println("Ready to accept connections at", addr)
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Fatal(err)
		}
		_, err = u.Upgrade(conn)
		if err != nil {
			log.Printf("upgrade error: %s", err)
			continue
		}

		// todo add connection to a map
		go func() {
			defer conn.Close()
			fmt.Println("new connection: ", conn.RemoteAddr())
			for {
				msg, op, err := wsutil.ReadClientData(conn)
				var closedError wsutil.ClosedError
				if errors.As(err, &closedError){
					fmt.Printf("%v broke connection", conn.RemoteAddr())
					break
				}
				if err != nil {
					fmt.Println("error in reading client data:", err)
					break
				}
				fmt.Println(op, string(msg))
				wsutil.WriteServerMessage(conn, op, msg)
			}
		}()
	}
}