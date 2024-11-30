package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	_ "github.com/joho/godotenv/autoload"
	"golang.org/x/crypto/scrypt"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"google.golang.org/api/option"
)

var secretKey []byte
var API_KEY string
var firestoreClient *firestore.Client

func init() {
	var set bool
	API_KEY, set = os.LookupEnv("API_KEY")
	if !set {
		panic("API_KEY env not set")
	}

	secretKeyStr, set := os.LookupEnv("SECRET_KEY")
	if !set {
		panic("SECRET_KEY env not set")
	}
	SALT, set := os.LookupEnv("SALT")
	if !set {
		panic("SALT env not set")
	}

	var err error
	// the values of N, r and p are the default ones nodejs crypto module uses
	// https://nodejs.org/api/crypto.html#cryptoscryptsyncpassword-salt-keylen-options
	secretKey, err = scrypt.Key([]byte(secretKeyStr), []byte(SALT), 16384, 8, 1, 32)
	if err != nil {
		panic("unable to create scrypt secret key: " + err.Error())
	}

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

func getToken(token string) (string, error) {
	snap, err := firestoreClient.
		Collection("tokens").
		Doc(token).
		Get(context.Background())

	if err != nil {
		return "", err
	}

	if !snap.Exists() {
		return "", fmt.Errorf("token doesnt exist")
	}

	return snap.Data()["token"].(string), nil
}

// checkAuth accepts the cookieValue and tries to authenticate the request.
// if successfull, it returns true and the token value.
func checkAuth(cookieValue []byte) (bool, string) {
	decryptedCookie, err := decrypt(string(cookieValue))
	if err != nil {
		fmt.Println("unable to decrypt cookie: ", err)
		return false, ""
	}

	var sessionCookie SessionCookie
	err = json.Unmarshal([]byte(decryptedCookie), &sessionCookie)
	if err != nil {
		fmt.Println("unable to decode cookie into struct", err)
		return false, ""
	}

	if sessionCookie.Token == "" {
		fmt.Println("session cookie token empty")
		return false, ""
	}

	dbToken, err := getToken(sessionCookie.Token)
	if err != nil {
		fmt.Println("error getting token from db", err)
		return false, ""
	}
	if dbToken != sessionCookie.Token {
		return false, ""
	}

	return true, dbToken
}