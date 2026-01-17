package store

import "sync"

// Testing new environment

type Store struct {
	sync.RWMutex // We use it instead of Mutex because we want to just lock the read
				 // not the entire variable, so many variables can read at the same time
				 // but none can write, using RLock()
	data map[string] any
}

func (s *Store) Get(key string) (any, bool) {
	s.RLock()
	defer s.RUnlock()

	val, ok := s.data[key]
	return val, ok
}

func (s *Store) Set(key string, value any) {
	// We use lock because we want to write here
	s.Lock()
	defer s.Unlock()

	s.data[key] = value
}

func (s *Store) Delete(key string) bool {
	s.Lock()
	defer s.Unlock()

	_, ok := s.data[key]
	if !ok {return false}
	delete(s.data, key)
	return true
}

func (s *Store) Exists(key string) bool {
	s.RLock()
	defer s.RUnlock()

	_, ok := s.data[key]
	return ok
}

func (s *Store) GetAll() map[string] any {
	s.RLock()
	defer s.RUnlock()
	// We create a copy instead of the actual map
	result := make(map[string]any)
    for key, value := range s.data {
        result[key] = value
    }
    return result
}