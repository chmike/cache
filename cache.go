package cache

import (
	"math/bits"
	"sync"
	"sync/atomic"
)

type item[K comparable, V any] struct {
	key   K      // item key
	value V      // item value
	bit   uint64 // bit mask
}

// Cache stores a finite number of key value pairs where keys are unique.
// Adding a new key value pair in a full cache result in overriding an
// existing key value pair using the second chance algorithm which yields
// efficiency similar to lru.
type Cache[K comparable, V any] struct {
	mu       sync.RWMutex
	idx      map[K]int       // index of keys to items
	items    []item[K, V]    // table of cached items
	bits     []atomic.Uint64 // bit map of ejectable items or free slots
	handIdx  int             // hand index in bits
	handMask uint64          // hand mask of bits to examine
	len      int             // number of used slots
}

// New instantiate a new cache with key of type K and value of type V.
// A key is unique and adding an existing key with a different value
// overrides the
// Size is the maximum number of
func New[K comparable, V any](size int) *Cache[K, V] {
	c := new(Cache[K, V])
	c.Init(size)
	return c
}

// Init initializes the cache.
func (c *Cache[K, V]) Init(size int) {
	c.mu.Lock()
	size = (size + 63) & ^63
	c.idx = make(map[K]int, size)
	c.items = make([]item[K, V], size)
	c.bits = make([]atomic.Uint64, size/64)
	c.handIdx = 0
	c.handMask = ^uint64(0)
	c.bits[0].Store(^uint64(0)) // initialize to all ejectable
	c.mu.Unlock()
}

// Reset resets the cache in the state it was just after Init.
func (c *Cache[K, V]) Reset() {
	c.mu.Lock()
	clear(c.idx)
	c.len = 0
	// cleanup items to avoid memory leak
	for i := range c.items {
		c.items[i] = item[K, V]{}
	}
	c.handIdx = 0
	c.handMask = ^uint64(0)
	c.mu.Unlock()
}

// Cap returns the maximum capacity of the cache.
func (c *Cache[K, V]) Cap() int {
	return cap(c.items)
}

// Len returns the number of key value pairs in the cache.
func (c *Cache[K, V]) Len() int {
	return c.len
}

// Get returns the value associated to the given key and true when it is found in the
// cache. Otherwise it returns false and the default value for the value type.
func (c *Cache[K, V]) Get(key K) (value V, ok bool) {
	c.mu.RLock()
	var idx int
	if idx, ok = c.idx[key]; ok {
		value = c.items[idx].value
		c.bits[idx/64].And(c.items[idx].bit)
	}
	c.mu.RUnlock()
	return
}

// Get returns the value associated to the given key and true when it is found in the
// cache. Otherwise it returns false and the default value for the value type.
func (c *Cache[K, V]) Has(key K) bool {
	c.mu.RLock()
	_, ok := c.idx[key]
	c.mu.RUnlock()
	return ok
}

// Add adds the key value pair to the cache. It returns false and the default value for
// the type when the pair could be inserted in a free slot. Otherwise it returns true and
// the overwritten or discarded value which may be recycled.
func (c *Cache[K, V]) Add(key K, value V) (oldValue V, ok bool) {
	c.mu.Lock()
	var idx int
	// if key already in cache
	if idx, ok = c.idx[key]; ok {
		// override value
		oldValue = c.items[idx].value
		c.items[idx].value = value
		c.bits[idx/64].And(c.items[idx].bit)
		c.mu.Unlock()
		return
	}
	// if cache not yet full, append item leaving hand unmodified
	if c.len < len(c.items) {
		c.items[c.len].key = key
		c.items[c.len].value = value
		c.items[c.len].bit = ^(uint64(1) << (c.len % 64))
		c.bits[c.len/64].And(c.items[c.len].bit)
		c.idx[key] = c.len
		c.len++
		c.mu.Unlock()
		return
	}

	// executed only when cache is full
	// locate the next element we can eject
	// set bits in handMask are the bits to check
	mbits := c.bits[c.handIdx].Load() & c.handMask
	if mbits == 0 {
		c.bits[c.handIdx].Or(c.handMask)
		c.handMask = ^uint64(0)
		if c.handIdx++; c.handIdx == len(c.bits) {
			c.handIdx = 0
		}
		for mbits = c.bits[c.handIdx].Load(); mbits == 0; mbits = c.bits[c.handIdx].Load() {
			c.bits[c.handIdx].Store(^uint64(0))
			if c.handIdx++; c.handIdx == len(c.bits) {
				c.handIdx = 0
			}
		}
	}
	// the less significant bit set in mbits is the element we eject
	bit := bits.TrailingZeros64(mbits)
	idx = c.handIdx*64 | bit

	oldValue = c.items[idx].value
	ok = true

	delete(c.idx, c.items[idx].key)
	c.idx[key] = idx

	c.items[idx].key = key
	c.items[idx].value = value
	c.bits[c.handIdx].And(c.items[idx].bit)

	if c.handMask = ^uint64(0) << (bit + 1); c.handMask == 0 {
		c.handMask = ^uint64(0)
		if c.handIdx++; c.handIdx == len(c.bits) {
			c.handIdx = 0
		}
	}

	c.mu.Unlock()
	return
}

// Delete returns the deleted value and true, when key is found in the cache to
// allow recycling the value. Otherwise, it returns the default value and false.
func (c *Cache[K, V]) Delete(key K) (value V, ok bool) {
	c.mu.Lock()
	var idx int
	if idx, ok = c.idx[key]; ok {
		value = c.items[idx].value
		delete(c.idx, key)
		c.len--
		if c.len != idx {
			// replace deleted item with last item
			c.idx[c.items[c.len].key] = idx
			c.items[idx].key = c.items[c.len].key
			c.items[idx].value = c.items[c.len].value
			c.items[c.len] = item[K, V]{}
			if c.bits[c.len/64].Load()&(^c.items[c.len].bit) == 0 {
				c.bits[idx/64].And(c.items[idx].bit)
			} else {
				c.bits[idx/64].Or(^c.items[idx].bit)
			}
		}
		// avoid memory leak
		c.items[c.len] = item[K, V]{}
	}
	c.mu.Unlock()
	return
}

// Items locks the cache and iterates over elements.
func (c *Cache[K, V]) Items() func(yield func(K, V) bool) {
	return func(yield func(key K, value V) bool) {
		c.mu.Lock()
		for k, idx := range c.idx {
			if !yield(k, c.items[idx].value) {
				break
			}
		}
		c.mu.Unlock()
	}
}
