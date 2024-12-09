package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"log"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/boltdb/bolt"
)


type storage struct {
	db *bolt.DB
	bucketName []byte
}

func NewStorage(filename, bucketName string) *storage {
	db, err := bolt.Open(filename, 0600, &bolt.Options{
		Timeout: 1 * time.Second,
	})

	if err != nil {
		log.Fatalf("unable to open boltdb: %v", err)
	}

	db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(bucketName))
		if err != nil {
			return fmt.Errorf("create bucket: %v", err)
		}
		return nil
	})

	return &storage{db: db, bucketName: []byte(bucketName)}
}

func (s *storage) AddSubscription(user string, sub webpush.Subscription) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(s.bucketName)
		err := b.Put([]byte(user), jsonify(sub))
		return err
	})
}

func getSubscriptionFromBytes(byteSlice []byte) webpush.Subscription {
	var sub webpush.Subscription
	if err := json.Unmarshal(byteSlice, &sub); err != nil {
		panic("unable to unmarshal byteslice into subscription")
	}
	return sub
}

func (s *storage) GetSubscription(user string) (webpush.Subscription, error) {
	byteSlice := []byte{}
	var emptySub webpush.Subscription

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(s.bucketName)
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

	return getSubscriptionFromBytes(byteSlice), nil
}

var errIterationEnd = errors.New("iteration has ended")

func (s *storage) GetAllSubscriptions() iter.Seq[webpush.Subscription] {
	return func(yield func(webpush.Subscription) bool) {
		s.db.View(func(tx *bolt.Tx) error {
			b := tx.Bucket(s.bucketName)
			b.ForEach(func(k, v []byte) error {
				if !yield(getSubscriptionFromBytes(v)) {
					return errIterationEnd
				}
				return nil
			})

			return nil
		})
	}
}