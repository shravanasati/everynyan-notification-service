package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"golang.org/x/crypto/scrypt"
)

type Config struct {
	SecretKey []byte
	API_KEY   string
}

func GetConfig() (*Config, error) {
	godotenv.Load("../.env")

	apiKey, set := os.LookupEnv("API_KEY")
	if !set {
		return nil, fmt.Errorf("API_KEY env not set")
	}

	secretKeyStr, set := os.LookupEnv("SECRET_KEY")
	if !set {
		return nil, fmt.Errorf("SECRET_KEY env not set")
	}
	salt, set := os.LookupEnv("SALT")
	if !set {
		return nil, fmt.Errorf("SALT env not set")
	}

	secretKey, err := processSecretKey([]byte(secretKeyStr), []byte(salt))
	if err != nil {
		return nil, err
	}

	conf := &Config{
		SecretKey: secretKey,
		API_KEY: apiKey,
	}

	return conf, nil
}

func MustConfig() *Config {
	c, e := GetConfig()
	if e != nil {
		panic(e)
	}

	return c
}

func processSecretKey(inputKey []byte, salt []byte) ([]byte, error) {
	// the values of N, r and p are the default ones nodejs crypto module uses
	// https://nodejs.org/api/crypto.html#cryptoscryptsyncpassword-salt-keylen-options
	secretKey, err := scrypt.Key(inputKey, salt, 16384, 8, 1, 32)
	if err != nil {
		return nil, fmt.Errorf("unable to create scrypt secret key: " + err.Error())
	}

	return secretKey, nil

}