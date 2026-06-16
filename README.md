<div align="center">
# hypermap-go

`hypermap-go` provides `hypermap.Map`: a compact, generic, insertion-ordered map for Go.

[![Go Reference](https://pkg.go.dev/badge/github.com/colduction/hypermap-go.svg)](https://pkg.go.dev/github.com/colduction/hypermap-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/colduction/hypermap-go)](https://goreportcard.com/report/github.com/colduction/hypermap-go)
![GitHub License](https://img.shields.io/github/license/Colduction/hypermap-go)

</div>

> [!TIP]
> Use `hypermap.New[K, V](capacity)` when the maximum live entry count is known; it reserves storage for steadier write performance.

## Install

```sh
go get -u github.com/colduction/hypermap-go@latest
```

## Usage

```go
package main

import (
	"fmt"

	"github.com/colduction/hypermap-go"
)

func main() {
	m := hypermap.New[string, int](3)

	m.Set("b", 2)
	m.Set("a", 1)
	m.Set("c", 3)
	m.MoveToFront("c")

	for key, value := range m.Range() {
		fmt.Println(key, value)
	}
}
```

## Features

| Capability          | Behavior                                                                  |
| ------------------- | ------------------------------------------------------------------------- |
| Zero value          | Ready to use without initialization.                                      |
| Ordering            | Preserves insertion order; replacing a value keeps the key in place.      |
| Lookup and mutation | O(1) average for lookup, insert, delete, movement, and front/back access. |
| Iteration           | Supports `for key, value := range m.Range()` with `iter.Seq2`.            |
| Storage reuse       | `Clear` keeps allocated storage, while `Reset` releases it.               |

> [!IMPORTANT]
> `Map` does not synchronize access. Share a map across goroutines only with external synchronization, or shard independent maps by key or worker for write-heavy services.

## License

This project is released under the MIT License. See [LICENSE](LICENSE).
