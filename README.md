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

> [!TIP]
> `Encode` allocates once: a `nil` or empty map returns `""`.

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

## Benchmarks

Median time with 4,096 `int`/`int` entries; lower is better and the fastest
result is bold.

| Operation       |     hypermap |          wk8 | elliotchance | lorenzosaino | vs best rival |
| --------------- | -----------: | -----------: | -----------: | -----------: | ------------: |
| Get             |     5.673 ns | **5.358 ns** |     5.364 ns |     5.407 ns |   5.9% slower |
| Replace         |     9.644 ns |     10.96 ns |     12.00 ns | **7.707 ns** |  25.1% slower |
| Delete + set    | **35.65 ns** |     90.98 ns |     62.38 ns |     74.91 ns |  42.8% faster |
| Move front/back | **14.32 ns** |     18.62 ns |            — |     17.27 ns |  17.1% faster |
| Range all       |     6.622 µs |     10.00 µs |     6.293 µs | **4.953 µs** |  33.7% slower |
| Fill new map    | **77.14 µs** |     175.2 µs |     120.2 µs |     268.9 µs |  35.8% faster |

`hypermap` is strongest under churn and construction: delete-and-set performs
with **0 B/op and 0 allocs/op** versus 32–56 B/op and 1–2 allocations for the
alternatives, while filling the map takes 19 allocations versus 4,114–8,213.

<details>
<summary>Methodology and reproduction</summary>

Results are medians from 10 one-second samples using Go 1.26.4 on Windows 11
`amd64` and an AMD Ryzen 9 7950X, restricted to one logical CPU. Capacity hints
are used where supported. `Range all` and `Fill new map` process all 4,096
entries. Replace timings had up to 20% sample variance.

Compared versions:

- [`wk8/go-ordered-map/v2` v2.1.8](https://github.com/wk8/go-ordered-map/tree/v2.1.8)
- [`elliotchance/orderedmap/v3` v3.1.0](https://github.com/elliotchance/orderedmap/tree/v3.1.0)
- [`lorenzosaino/go-orderedmap` cf642d9](https://github.com/lorenzosaino/go-orderedmap/commit/cf642d91fab6)

The source and pinned dependency versions are in [`benchmarks`](benchmarks):

```sh
cd benchmarks
go test -run '^$' -bench . -benchmem -benchtime=1s -count=10 -cpu=1
```

Microbenchmark results vary with workload, Go version, and hardware.

</details>

## License

This project is released under the MIT License. See [LICENSE](LICENSE).
