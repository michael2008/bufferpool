package bufferpool

import (
	"fmt"
	"math/rand"
	"runtime"
	"testing"
	"time"
)

func TestPoolFreshAlloc(t *testing.T) {
	// when there is no Put back,
	// Get should return a new buffer
	// with a buffer containing the requested size.
	type testCase struct {
		rSize int  // requested buffer size
		bSize int  // expected buffer size
		equal bool // if true, the buffer size should be equal to the requested size
	}
	var data []testCase
	for i, size := range defaultPoolSizes {
		// requested size is equal to the size of the sub-pool
		data = append(data, testCase{
			rSize: size,
			bSize: size,
			equal: true,
		})
		// requested size is smaller than the size of the sub-pool
		data = append(data, testCase{
			rSize: size - 1,
			bSize: size,
			equal: true,
		})
		// requested size is larger than the size of the sub-pool except the last sub-pool
		if i < len(defaultPoolSizes)-1 {
			data = append(data, testCase{
				rSize: size + 1,
				bSize: defaultPoolSizes[i+1],
				equal: true,
			})
		}
		// requested size is larger than the size of the last sub-pool
		if i == len(defaultPoolSizes)-1 {
			data = append(data, testCase{
				rSize: size + 100,
				bSize: size + 100,
				equal: false,
			})
		}
	}
	// size <= 0
	data = append(data, testCase{
		rSize: 0,
		bSize: 0,
		equal: true,
	})
	data = append(data, testCase{
		rSize: -1,
		bSize: 0,
		equal: true,
	})
	for _, tc := range data {
		t.Run(fmt.Sprintf("rSize=%d, bSize=%d", tc.rSize, tc.bSize), func(t *testing.T) {
			buf := Get(tc.rSize)
			if tc.equal {
				if buf.Cap() != tc.bSize {
					t.Errorf("expected buffer to have capacity %d, got %d", tc.bSize, buf.Cap())
				}
			} else {
				if buf.Cap() <= tc.bSize {
					t.Errorf("expected buffer to have capacity larger than %d, got %d", tc.bSize, buf.Cap())
				}
			}
			// no putting back
		})
	}
}

func TestPoolStressByteSlicePool(t *testing.T) {
	var p BufferPool

	const P = 10
	chs := 1000
	maxSize := 1 << 16
	N := int(1e4)
	if testing.Short() {
		N /= 100
	}
	done := make(chan bool)
	errs := make(chan error)
	for i := 0; i < P; i++ {
		go func() {
			ch := make(chan *Buffer, chs+1)

			for i := 0; i < chs; i++ {
				j := rand.Int() % maxSize
				ch <- p.Get(j)
			}

			for j := 0; j < N; j++ {
				r := 0
				for i := 0; i < chs; i++ {
					v := <-ch
					p.Put(v)
					r = rand.Int() % maxSize
					v = p.Get(r)
					if v.Cap() < r {
						errs <- fmt.Errorf("expect Cap(v) >= %d, got %d", j, v.Cap())
					}
					ch <- v
				}

				if r%1000 == 0 {
					runtime.GC()
				}
			}
			done <- true
		}()
	}

	for i := 0; i < P; {
		select {
		case <-done:
			i++
		case err := <-errs:
			t.Error(err)
		}
	}
}

func BenchmarkPool(b *testing.B) {
	var p BufferPool
	b.RunParallel(func(pb *testing.PB) {
		i := 7
		for pb.Next() {
			if i > 1<<20 {
				i = 7
			} else {
				i = i << 1
			}
			buf := p.Get(i)
			buf.Buffer.WriteByte(byte(i))
			p.Put(buf)
		}
	})
}

func BenchmarkAlloc(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		i := 7
		for pb.Next() {
			if i > 1<<20 {
				i = 7
			} else {
				i = i << 1
			}
			b := make([]byte, i)
			b[1] = byte(i)
		}
	})
}

func TestPoolVariousSizesSerial(t *testing.T) {
	testPoolVariousSizes(t)
}

func TestPoolVariousSizesConcurrent(t *testing.T) {
	concurrency := 5
	ch := make(chan struct{})
	for i := 0; i < concurrency; i++ {
		go func() {
			testPoolVariousSizes(t)
			ch <- struct{}{}
		}()
	}
	for i := 0; i < concurrency; i++ {
		select {
		case <-ch:
		case <-time.After(10 * time.Second):
			t.Fatalf("timeout")
		}
	}
}

func testPoolVariousSizes(t *testing.T) {
	steps := 26
	for i := 0; i < steps+1; i++ {
		n := 1 << uint32(i)

		testGetPut(t, n)
		testGetPut(t, n+1)
		testGetPut(t, n-1)

		for j := 0; j < 10; j++ {
			testGetPut(t, j+n)
		}
	}
}

func testGetPut(t *testing.T, n int) {
	bb := Get(n)
	if bb.Len() > 0 {
		t.Fatalf("non-empty byte buffer returned from acquire")
	}
	allocNBytes(bb, n)
	Put(bb)
}

func allocNBytes(b *Buffer, n int) {
	for i := 0; i < n; i++ {
		b.WriteByte(byte(i))
	}
}
