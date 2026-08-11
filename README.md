<div align="center">

# hypermap-go

`hypermap-go` provides `hypermap.Map`: a compact, generic, insertion-ordered map for Go.

[![Go Reference](https://pkg.go.dev/badge/github.com/colduction/hypermap-go.svg)](https://pkg.go.dev/github.com/colduction/hypermap-go)
![GitHub License](https://img.shields.io/github/license/Colduction/hypermap-go)

</div>

> [!TIP]
> Use `hypermap.New[K, V](capacity)` when the maximum live entry count is known; it avoids growth during the initial fill. Different-key churn may still grow the hash index to preserve amortized write cost.

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
> `Encode` allocates once when it emits output. A nil/empty map, or one whose value slices are all empty, returns `""` without that output allocation.

## Features

| Capability          | Behavior                                                                       |
| ------------------- | ------------------------------------------------------------------------------ |
| Zero value          | Ready to use without initialization.                                           |
| Ordering            | Preserves insertion order; replacing a value keeps the key in place.           |
| Lookup and mutation | O(1) average for lookup, insert, delete, movement, and front/back access.      |
| Replacement         | `Replace` updates only an existing key; `Set` inserts or replaces.             |
| Iteration           | `Range` provides `iter.Seq2`; `RangeFunc` is the lower-overhead callback form. |
| Storage reuse       | `Clear` keeps allocated storage, while `Reset` releases it.                    |
| Query encoding      | `QueryMap.Encode` renders `string`/`[]string` entries as a query string.       |

> [!IMPORTANT]
> Do not copy a `Map` after initialization or mutation. It does not synchronize access; share one across goroutines only with external synchronization, or shard independent maps by key or worker for write-heavy services.

## Benchmarks

Median time with 4,096 `int`/`int` entries; lower is better. These are the
maintained headline workloads, and the fastest result in each row is bold.

| Operation       |     hypermap |      wk8 | elliotchance | lorenzosaino | vs best rival |
| --------------- | -----------: | -------: | -----------: | -----------: | ------------: |
| Get             | **4.799 ns** | 5.413 ns |     5.457 ns |     5.480 ns |  11.3% faster |
| Replace         | **5.897 ns** | 10.46 ns |     12.10 ns |     7.751 ns |  23.9% faster |
| Delete + set    | **24.14 ns** | 91.35 ns |     63.05 ns |     75.04 ns |  61.7% faster |
| Move front/back | **16.03 ns** | 27.07 ns |            — |     27.34 ns |  40.8% faster |
| Range all       | **4.030 µs** | 10.13 µs |     4.653 µs |     4.863 µs |  13.4% faster |
| Fill new map    | **53.51 µs** | 175.4 µs |     119.9 µs |     267.4 µs |  55.4% faster |

Every operation on an already-populated Hypermap in the table is **0 B/op and
0 allocs/op**.
Filling a capacity-sized map uses 147,456 B and 2 allocations, versus
278,896–492,776 B and 4,114–8,213 allocations for the alternatives. The broader
suite also measures misses, strings, tiny maps, fragmented traversal, early
stop, unhinted construction, and different-key churn.

<details>
<summary>Methodology and reproduction</summary>

Results are medians from 10 one-second samples using Go 1.26.5 on Windows 11
`amd64` and an AMD Ryzen 9 7950X, pinned to logical CPU 2 with `GOMAXPROCS=1`.
Capacity hints are used where supported. `Range all` and `Fill new map` process
all 4,096 entries. Replacement and traversal use each package's fastest
non-allocating API: Hypermap uses `Replace` and `RangeFunc`. Timings can vary
with map seed and system clock state; the table reports ten-sample medians, not
universal dominance.

Compared versions:

- [`wk8/go-ordered-map/v2` v2.1.8](https://github.com/wk8/go-ordered-map/tree/v2.1.8)
- [`elliotchance/orderedmap/v3` v3.1.0](https://github.com/elliotchance/orderedmap/tree/v3.1.0)
- [`lorenzosaino/go-orderedmap` cf642d9](https://github.com/lorenzosaino/go-orderedmap/commit/cf642d91fab6)

The source, validation checks, and pinned dependency versions are in
[`benchmarks`](benchmarks). The maintained Windows headline command is:

```powershell
cd benchmarks
$benchProcess = Get-Process -Id $PID
$benchProcess.ProcessorAffinity = [IntPtr]4
$benchProcess.PriorityClass = 'High'
$headline = '^(BenchmarkGet|BenchmarkSetReplace|BenchmarkDeleteSet|BenchmarkMoveToFrontBack|BenchmarkRange|BenchmarkFill)$'
go test -run '^$' -bench $headline -benchmem -benchtime=100ms -count=1 -cpu=1 | Out-Null
go test -run '^$' -bench $headline -benchmem -benchtime=1s -count=10 -cpu=1
```

Run the broader workload matrix with `-bench '^BenchmarkWorkload'`. Built-in
map fast paths can still win for some 8- or 64-entry operations, so the table is
not a claim of universal superiority across all key types, sizes, or machines.
See [the performance design and tradeoffs](docs/performance.md) for the data
layout, research basis, allocation caveats, and complete protocol.

</details>

## License

This project is released under the MIT License. See [LICENSE](LICENSE).
