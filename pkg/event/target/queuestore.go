/*
 * MinIO Cloud Storage, (C) 2019 MinIO, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package target

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/minio/minio/pkg/event"
)

const (
	maxLimit = 10000 // Max store limit.
	eventExt = ".event"
)

// QueueStore - Filestore for persisting events.
type QueueStore struct {
	sync.RWMutex
	directory string
	eC        uint16
	limit     uint16
}

// NewQueueStore - Creates an instance for QueueStore.
func NewQueueStore(directory string, limit uint16) *QueueStore {
	if limit == 0 {
		limit = maxLimit
	}
	queueStore := &QueueStore{
		directory: directory,
		limit:     limit,
	}
	return queueStore
}

// Open - Creates the directory if not present.
func (store *QueueStore) Open() error {
	store.Lock()
	defer store.Unlock()

	if terr := os.MkdirAll(store.directory, os.FileMode(0770)); terr != nil {
		return terr
	}

	eCount := uint16(len(store.listAll()))
	if eCount >= store.limit {
		return ErrLimitExceeded
	}

	store.eC = eCount

	return nil
}

// write - writes event to the directory.
func (store *QueueStore) write(directory string, key string, e event.Event) error {

	// Marshalls the event.
	eventData, err := json.Marshal(e)
	if err != nil {
		return err
	}

	path := filepath.Join(store.directory, key+eventExt)
	if err := ioutil.WriteFile(path, eventData, os.FileMode(0770)); err != nil {
		return err
	}

	// Increment the event count.
	store.eC++

	return nil
}

// Put - puts a event to the store.
func (store *QueueStore) Put(e event.Event) error {
	store.Lock()
	defer store.Unlock()
	if store.eC >= store.limit {
		return ErrLimitExceeded
	}
	key, kErr := getNewUUID()
	if kErr != nil {
		return kErr
	}
	return store.write(store.directory, key, e)
}

// Get - gets a event from the store.
func (store *QueueStore) Get(key string) (event.Event, error) {
	store.RLock()
	defer store.RUnlock()

	var event event.Event

	filepath := filepath.Join(store.directory, key+eventExt)

	eventData, rerr := ioutil.ReadFile(filepath)
	if rerr != nil {
		store.del(key)
		return event, rerr
	}

	if len(eventData) == 0 {
		store.del(key)
	}

	uerr := json.Unmarshal(eventData, &event)
	if uerr != nil {
		store.del(key)
		return event, uerr
	}

	return event, nil
}

// Del - Deletes an entry from the store.
func (store *QueueStore) Del(key string) {
	store.Lock()
	defer store.Unlock()
	store.del(key)
}

// lockless call
func (store *QueueStore) del(key string) {
	p := filepath.Join(store.directory, key+eventExt)

	rerr := os.Remove(p)
	if rerr != nil {
		return
	}

	// Decrement the event count.
	store.eC--
}

// ListAll - lists all the keys in the directory.
func (store *QueueStore) ListAll() []string {
	store.RLock()
	defer store.RUnlock()
	return store.listAll()
}

// lockless call.
func (store *QueueStore) listAll() []string {
	var err error
	var keys []string
	var files []os.FileInfo

	files, err = ioutil.ReadDir(store.directory)
	if err != nil {
		return nil
	}

	for _, f := range files {
		keys = append(keys, strings.TrimSuffix(f.Name(), eventExt))
	}
	return keys
}
