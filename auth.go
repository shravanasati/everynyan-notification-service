package main

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	_ "github.com/joho/godotenv/autoload"
	"golang.org/x/crypto/scrypt"
)

var secretKey []byte

func init() {
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
}

func decrypt(encryptedText string) (string, error) {
	encryptedText = strings.ReplaceAll(encryptedText, "%2F", "/")
	encryptedText = strings.ReplaceAll(encryptedText, "%3D", "=")
	encryptedText = strings.ReplaceAll(encryptedText, "%2B", "+")

	// Decode the base64-encoded data
	data, err := base64.StdEncoding.DecodeString(encryptedText)
	if err != nil {
		return "", err
	}

	// Extract the nonce (12 bytes after the prefix)
	nonce := data[3:15] // Skip the "v10" prefix
	ciphertext := data[15:] // Rest is ciphertext + auth tag

	// Create AES block
	block, err := aes.NewCipher([]byte(secretKey))
	if err != nil {
		return "", err
	}

	// Create GCM
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	// Decrypt the ciphertext
	plainText, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plainText), nil
}

func checkAuth(cookieValue []byte) bool {
	decryptedCookie, err := decrypt(string(cookieValue))
	if err != nil {
		fmt.Println("unable to decrypt cookie: ", err)
		return false
	}

	// todo json parse this and then ping firestore or something
	fmt.Println(decryptedCookie)
	return true
}
