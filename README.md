# About
This is varying-sized buffer pool wrapping for bytes.Buffer.
It manages sub-pools of varying sizes to optimize memory usage.
It is suitable for callers that knows the exact or approximate size of desired buffers.

# Example

## Default Pool
```go
package main

import (
	"fmt"
	"github.com/michael2008/bufferpool"
)

func main() {
	// desired buffer size: exact or approximate
	data := "hello world"
	size := len(data)
	buf := bufferpool.Get(size)
	// do something with bytes.Buffer
	buf.WriteString(data)
	fmt.Println(buf.String())
	// put back to the pool
	bufferpool.Put(buf)
	// MUST NOT use the buf after Put
}
```

## Custom Pool
```go
package main

import (
	"fmt"
	"github.com/michael2008/bufferpool"
)

func main() {
	// create a custom pool with the specified sizes.
	const kb = 1024
	const mb = kb * 1024
	sizes := []int{kb * 512, mb * 1, mb * 16, mb * 32}
	pool := bufferpool.MustNew(sizes)
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
```

# Rationale
The lib is inspired by https://github.com/libp2p/go-buffer-pool. It uses 32 sub-pools of
varying sizes from the power of 2 in 0-32. This may not be ideal for small and large buffers.
So this lib chooses a more common size range of 8KiB-64MiB, which is also customizable.

# Requirements
Go 1.18 or later.
