package bufferpool

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
)

const (
	kilo = 1024
	mega = 1024 * kilo
)

// defaultPoolSizes defines the default sizes of each sub-pool in bytes
var defaultPoolSizes = []int{
	8 * kilo,   // 8KB
	32 * kilo,  // 32KB
	128 * kilo, // 128KB
	512 * kilo, // 512KB
	mega,       // 1MB
	2 * mega,   // 2MB
	4 * mega,   // 4MB
	8 * mega,   // 8MB
	16 * mega,  // 16MB
	32 * mega,  // 32MB
	64 * mega,  // 64MB
}

var _once sync.Once

// globalPool is a static Pool for reusing byte buffers of various sizes.
var globalPool = MustNew(defaultPoolSizes)

// MaxLength is the maximum length of sub pool size.
// Generally, storing an excessively large element in the Pool will consume
// more memory.
const MaxLength = math.MaxInt32

// MinLength is the allowed minimum of sub pool size.
// It does not make sense to cache extremely tiny buffers.
const MinLength = 128

// Get retrieves a buffer of the appropriate capacity from the global buffer pool
// (or allocates a new one).
func Get(length int) *Buffer {
	_once.Do(func() {
		globalPool = MustNew(defaultPoolSizes)
	})
	return globalPool.Get(length)
}

// Put returns a buffer to the global buffer pool.
func Put(buf *Buffer) {
	_once.Do(func() {
		globalPool = MustNew(defaultPoolSizes)
	})
	globalPool.Put(buf)
}

// Pool wraps [pkg/sync.Pool].
type Pool[T any] struct {
	New     func() T
	p       sync.Pool
	newOnce sync.Once
}

func (p *Pool[T]) init() {
	p.newOnce.Do(func() {
		p.p.New = func() any {
			if p.New == nil {
				var zero T
				return zero
			}
			return p.New()
		}
	})
}

// Buffer is a wrapper around bytes.Buffer.
type Buffer struct {
	*bytes.Buffer
	rSize int
}

// Get wraps [pkg/sync.Pool.Get].
func (p *Pool[T]) Get() T {
	p.init()
	return p.p.Get().(T)
}

// Put wraps [pkg/sync.Pool.Put].
func (p *Pool[T]) Put(x T) {
	p.init()
	p.p.Put(x)
}

// BufferPool is a pool to handle cases of reusing elements of varying sizes.
// It maintains sub-pools for specific buffer sizes.
//
// As there may be no generic buffer pool that is optimal for all use cases,
// this package provides a buffer pool suitable for vary-sized buffers especially
// for callers that knows the exact or approximate size of desired buffers.
//
// You should generally just call the package level Get and Put methods
// instead of constructing your own, except that you need to customize the
// sizes of the sub-pools. The default sizes range from 8KiB to 64MiB which
// should cover most use cases.
type BufferPool struct {
	once sync.Once
	// Sub-pools with predefined sizes
	pools []Pool[*bytes.Buffer]
	// Sorted sizes for each pool
	sizes []int
	// A pool for unknown size
	any *Pool[*bytes.Buffer]
}

func (p *BufferPool) init() {
	p.once.Do(func() {
		if len(p.sizes) == 0 {
			p.sizes = defaultPoolSizes
		}
		// Create pools for each size
		pools := make([]Pool[*bytes.Buffer], len(p.sizes))
		for i := range pools {
			pools[i].New = func() *bytes.Buffer {
				return bytes.NewBuffer(make([]byte, 0, p.sizes[i]))
			}
		}
		ap := &Pool[*bytes.Buffer]{
			New: func() *bytes.Buffer {
				return bytes.NewBuffer(nil)
			},
		}
		p.any = ap
		p.pools = pools
	})
}

// New creates a new BufferPool with the specified sizes.
// Sizes are better to be powers of 2.
//
// The sizes will be sorted and deduplicated.
// At least one size must be provided.
// Size should not exceed the MaxLength and not be smaller than the MinLength.
func New(sizes []int) (*BufferPool, error) {
	if len(sizes) == 0 {
		return nil, errors.New("at least one pool size must be provided")
	}

	// Sort and deduplicate sizes
	sortedSizes := make([]int, 0, len(sizes))
	seen := make(map[int]bool)

	for _, size := range sizes {
		if size <= 0 {
			return nil, errors.New("pool sizes must be positive")
		}
		if size > MaxLength {
			return nil, fmt.Errorf("buffer size %d exceeds the maximum length %d", size, MaxLength)
		}
		if size < MinLength {
			return nil, fmt.Errorf("buffer size %d is smaller than the minimum length %d", size, MinLength)
		}
		if !seen[size] {
			sortedSizes = append(sortedSizes, size)
			seen[size] = true
		}
	}
	sort.Ints(sortedSizes)

	p := &BufferPool{
		sizes: sortedSizes,
	}
	p.init()
	return p, nil
}

// MustNew creates a new BufferPool with the specified sizes.
// It panics if there's an error.
func MustNew(sizes []int) *BufferPool {
	pool, err := New(sizes)
	if err != nil {
		panic(err)
	}
	return pool
}

// findPoolIndex returns the index of the smallest pool that can hold a buffer of the given length.
// Return -1 if no suitable pool found.
func (p *BufferPool) findPoolIndex(length int) int {
	idx := sort.SearchInts(p.sizes, length)
	if idx < len(p.sizes) {
		return idx
	}
	return -1
}

// Get retrieves a buffer of the appropriate capacity from the buffer pool or
// allocates a new one. Get may choose to ignore the pool and treat it as empty.
// Callers should not assume any relation between values passed to Put and the
// values returned by Get.
//
// If no suitable buffer exists in the pool, Get creates one.
//
// Special cases:
//   - if length <=0 (meaning the size is unknown), Get returns a buffer from an internal dedicated pool.
//   - if length is larger than the max sub pool size, Get returns a new buffer
//     than can hold the given length. note that the buffer won't be put back to the pool when calling Put.
func (p *BufferPool) Get(length int) (buffer *Buffer) {
	p.init()
	buffer = &Buffer{Buffer: bytes.NewBuffer(nil), rSize: length}
	if length <= 0 {
		if buf := p.any.Get(); buf != nil {
			buf.Reset()
			buffer.Buffer = buf
		}
		return
	}
	// Find the appropriate pool index
	idx := p.findPoolIndex(length)
	if idx < 0 {
		// If the requested size is larger than our largest pool, just allocate a new buffer
		// that can hold the requested size.
		buffer.Grow(length)
		return
	}
	// Try to get a buffer from the pool
	buf := p.pools[idx].Get()
	if buf == nil {
		// this should not happen, but just in case
		buffer.Grow(p.sizes[idx])
		return
	}

	// Reset the buffer and ensure it has the right capacity
	buf.Reset()
	buffer.Buffer = buf
	return
}

// Put adds a buffer to the pool.
// The buffer will be reset and MUST NOT be used after this.
//
// If the buffer is larger than the max sub pool size or smaller than
// the min sub pool size, Put will discard it.
//
// The returned buffer may be GCed by golang runtime at any time.
func (p *BufferPool) Put(buf *Buffer) {
	if buf == nil || buf.Buffer == nil {
		return
	}
	p.init()

	capacity := buf.Cap()
	if capacity == 0 || capacity > p.sizes[len(p.sizes)-1] {
		return // drop it
	}

	if buf.rSize <= 0 {
		// Put the buffer back in the dedicated pool.
		buf.Reset()
		p.any.Put(buf.Buffer)
	} else {
		// Find the pool that would have created this buffer.
		// We need to find the pool with size <= capacity that is closest to capacity
		idx := -1
		for i := len(p.sizes) - 1; i >= 0; i-- {
			if p.sizes[i] <= capacity {
				idx = i
				break
			}
		}
		if idx == -1 {
			return // Buffer is smaller than the smallest pool, drop it
		}

		// Reset the buffer before returning it to the pool
		buf.Reset()

		// Put the buffer back in the pool
		p.pools[idx].Put(buf.Buffer)
	}
}
