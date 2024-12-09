package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/boltdb/bolt"
)

var bucketName = []byte("subscriptions")

type storage struct {
	db *bolt.DB
}

func NewStorage(filename string) *storage {
	db, err := bolt.Open(filename, 0600, &bolt.Options{
		Timeout: 1 * time.Second,
	})

	if err != nil {
		log.Fatalf("unable to open boltdb: %v", err)
	}

	db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketName)
		if err != nil {
			return fmt.Errorf("create bucket: %v", err)
		}
		return nil
	})

	return &storage{db: db}
}

func (s *storage) AddSubscription(user string, sub webpush.Subscription) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		err := b.Put([]byte(user), jsonify(sub))
		return err
	})
}

func (s *storage) GetSubscription(user string) (webpush.Subscription, error) {
	byteSlice := []byte{}
	var emptySub webpush.Subscription

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		v := b.Get([]byte(user))
		if v != nil {
			byteSlice = append(byteSlice, v...)
			return nil
		}
		return fmt.Errorf("not found")
	})
	if err != nil {
		return emptySub, err
	}

	var sub webpush.Subscription
	if err := json.Unmarshal(byteSlice, &sub); err != nil {
		return emptySub, err
	}

	return sub, nil
}
