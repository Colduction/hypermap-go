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

## URL query encoding

`hypermap.QueryMap` is a `Map[string, []string]` with an `Encode` method that
renders the entries as a URL query string in the current key order. It embeds
`Map`, so every map method (`Set`, `Get`, `MoveToFront`, `Range`, …) is
available on it as well.

```go
m := hypermap.NewQueryMap(3)

m.Set("name", []string{"ada lovelace"})
m.Set("tags", []string{"math", "code"})

fmt.Println(m.Encode()) // name=ada+lovelace&tags=math&tags=code
```

`Encode` allocates once: a `nil` or empty map returns `""`, and keys with no
values are skipped.

## Features

| Capability          | Behavior                                                                  |
| ------------------- | ------------------------------------------------------------------------- |
| Zero value          | Ready to use without initialization.                                      |
| Ordering            | Preserves insertion order; replacing a value keeps the key in place.      |
| Lookup and mutation | O(1) average for lookup, insert, delete, movement, and front/back access. |
| Iteration           | Supports `for key, value := range m.Range()` with `iter.Seq2`.            |
| Storage reuse       | `Clear` keeps allocated storage, while `Reset` releases it.               |
| Query encoding      | `QueryMap.Encode` renders `string`/`[]string` entries as a query string.  |

> [!IMPORTANT]
> `Map` does not synchronize access. Share a map across goroutines only with external synchronization, or shard independent maps by key or worker for write-heavy services.

## License

This project is released under the MIT License. See [LICENSE](LICENSE).
