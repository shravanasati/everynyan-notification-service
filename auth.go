package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	_ "github.com/joho/godotenv/autoload"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"google.golang.org/api/option"
)

var secretKey []byte
var API_KEY string
var firestoreClient *firestore.Client

func init() {
	opt := option.WithCredentialsFile("./serviceAccountKey.json")
	app, err := firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		panic("unable to initialize firebase: " + err.Error())
	}

	firestoreClient, err = app.Firestore(context.Background())
	if err != nil {
		panic("unable to initialize firestore: " + err.Error())
	}
}

func decrypt(encryptedText string) ([]byte, error) {
	encryptedText = strings.ReplaceAll(encryptedText, "%2F", "/")
	encryptedText = strings.ReplaceAll(encryptedText, "%3D", "=")
	encryptedText = strings.ReplaceAll(encryptedText, "%2B", "+")
	emptyByteSlice := []byte{}

	// Decode the base64-encoded data
	data, err := base64.StdEncoding.DecodeString(encryptedText)
	if err != nil {
		return emptyByteSlice, err
	}

	// Extract the nonce (12 bytes after the prefix)
	nonce := data[3:15]     // Skip the "v10" prefix
	ciphertext := data[15:] // Rest is ciphertext + auth tag

	// Create AES block
	block, err := aes.NewCipher([]byte(secretKey))
	if err != nil {
		return emptyByteSlice, err
	}

	// Create GCM
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return emptyByteSlice, err
	}

	// Decrypt the ciphertext
	plainText, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return emptyByteSlice, err
	}

	return (plainText), nil
}

type SessionCookie struct {
	Token string `json:"token"`
	Role  string `json:"role"`
}

func getToken(token string) (SessionCookie, error) {
	snap, err := firestoreClient.
		Collection("tokens").
		Doc(token).
		Get(context.Background())

	if err != nil {
		return SessionCookie{}, err
	}

	if !snap.Exists() {
		return SessionCookie{}, fmt.Errorf("token doesnt exist")
	}

	data := snap.Data()
	return SessionCookie{
		Token: data["token"].(string),
		Role:  data["role"].(string),
	}, nil
}

// checkAuth accepts the cookieValue and tries to authenticate the request.
// if successfull, it returns true and the token value.
func checkAuth(cookieValue []byte) (bool, SessionCookie) {
	var emptyCookie SessionCookie
	decryptedCookie, err := decrypt(string(cookieValue))
	if err != nil {
		fmt.Println("unable to decrypt cookie: ", err)
		return false, emptyCookie
	}

	var sessionCookie SessionCookie
	err = json.Unmarshal([]byte(decryptedCookie), &sessionCookie)
	if err != nil {
		fmt.Println("unable to decode cookie into struct", err)
		return false, emptyCookie
	}

	if sessionCookie.Token == "" {
		fmt.Println("session cookie token empty")
		return false, emptyCookie
	}

	dbToken, err := getToken(sessionCookie.Token)
	if err != nil {
		fmt.Println("error getting token from db", err)
		return false, emptyCookie
	}
	if dbToken.Token != sessionCookie.Token {
		return false, emptyCookie
	}

	return true, dbToken
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
