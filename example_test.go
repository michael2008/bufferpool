package bufferpool

import (
	"fmt"
	"testing"
)

func TestPool(t *testing.T) {
	// desired buffer size: exact or approximate
	data := "hello world"
	size := len(data)
	buf := Get(size)
	// do something with bytes.Buffer
	buf.WriteString(data)
	fmt.Println(buf.String())
	// put back to the pool
	Put(buf)
	// MUST NOT use the buf after Put
}

func TestCustomPool(t *testing.T) {
	// create a custom pool with the specified sizes.
	const kb = 1024
	const mb = kb * 1024
	sizes := []int{kb * 512, mb * 1, mb * 16, mb * 32}
	pool := MustNew(sizes)
	// desired buffer size: exact or approximate
	data := "hello world"
	size := len(data)
	buf := pool.Get(size)
	// do something with bytes.Buffer
	buf.WriteString(data)
	fmt.Println(buf.String())
	// put back to the pool
	pool.Put(buf)
	// MUST NOT use the buf after Put
}
