package services

import (
	"github.com/dgraph-io/badger"

	"github.com/lucmichalski/gopress/pkg/utils"
)

var	(
	KV *badger.DB
)

func InitBadger(storagePath string) (*badger.DB, error) {
	err = utils.EnsureDir(storagePath)
	if err != nil {
		return nil, err
	}
	store, err = badger.Open(badger.DefaultOptions(storagePath))
	if err != nil {
		return nil, err
	}
	return store, nil
}

func getFromBadger(key string) (resp []byte, ok bool) {
	err := store.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}
		err = item.Value(func(val []byte) (err error) {
			resp, err = decompress(val)
			if err != nil {
				return err
			}
			// This func with val would only be called if item.Value encounters no error.
			// Accessing val here is valid.
			// fmt.Printf("The answer is: %s\n", val)
			return nil
		})
		if err != nil {
			return err
		}
		return nil
	})
	return resp, err == nil
}

func addToBadger(key, value string) error {
	err := store.Update(func(txn *badger.Txn) error {
		if debug {
			log.Println("indexing: ", key)
		}
		cnt, err := compress([]byte(value))
		if err != nil {
			return err
		}
		err = txn.Set([]byte(key), cnt)
		return err
	})
	return err
}

func compress(data []byte) ([]byte, error) {
	return snappy.Encode([]byte{}, data), nil
}

func decompress(data []byte) ([]byte, error) {
	return snappy.Decode([]byte{}, data)
}