// Package orderedmapbench compares insertion-ordered map implementations.
package orderedmapbench

import (
	"testing"

	hypermap "github.com/colduction/hypermap-go"
	elliott "github.com/elliotchance/orderedmap/v3"
	lorenzo "github.com/lorenzosaino/go-orderedmap"
	wk8 "github.com/wk8/go-ordered-map/v2"
)

const (
	benchmarkSize = 4096
	benchmarkMask = benchmarkSize - 1
)

var (
	benchmarkValue int
	benchmarkOK    bool
	benchmarkErr   error
	benchmarkLen   int
)

// BenchmarkGet measures lookup in a populated ordered map.
func BenchmarkGet(b *testing.B) {
	b.Run("hypermap", benchmarkHypermapGet)
	b.Run("wk8", benchmarkWK8Get)
	b.Run("elliotchance", benchmarkElliottGet)
	b.Run("lorenzosaino", benchmarkLorenzoGet)
}

// BenchmarkSetReplace measures each package's replacement-only fast path when
// available, without changing key order.
func BenchmarkSetReplace(b *testing.B) {
	b.Run("hypermap", benchmarkHypermapSetReplace)
	b.Run("wk8", benchmarkWK8SetReplace)
	b.Run("elliotchance", benchmarkElliottSetReplace)
	b.Run("lorenzosaino", benchmarkLorenzoSetReplace)
}

// BenchmarkDeleteSet measures steady-state delete and append churn.
func BenchmarkDeleteSet(b *testing.B) {
	b.Run("hypermap", benchmarkHypermapDeleteSet)
	b.Run("wk8", benchmarkWK8DeleteSet)
	b.Run("elliotchance", benchmarkElliottDeleteSet)
	b.Run("lorenzosaino", benchmarkLorenzoDeleteSet)
}

// BenchmarkMoveToFrontBack measures two effective relinks of one existing key:
// back/middle to front, then front to back.
func BenchmarkMoveToFrontBack(b *testing.B) {
	b.Run("hypermap", benchmarkHypermapMoveToFrontBack)
	b.Run("wk8", benchmarkWK8MoveToFrontBack)
	b.Run("lorenzosaino", benchmarkLorenzoMoveToFrontBack)
}

// BenchmarkRange measures each package's fastest non-allocating complete
// insertion-order traversal API.
func BenchmarkRange(b *testing.B) {
	b.Run("hypermap", benchmarkHypermapRange)
	b.Run("wk8", benchmarkWK8Range)
	b.Run("elliotchance", benchmarkElliottRange)
	b.Run("lorenzosaino", benchmarkLorenzoRange)
}

// BenchmarkFill measures constructing and filling a capacity-sized map.
func BenchmarkFill(b *testing.B) {
	b.Run("hypermap", benchmarkHypermapFill)
	b.Run("wk8", benchmarkWK8Fill)
	b.Run("elliotchance", benchmarkElliottFill)
	b.Run("lorenzosaino", benchmarkLorenzoFill)
}

func benchmarkHypermapGet(b *testing.B) {
	m := newHypermap()
	var (
		value int
		ok    bool
		i     int
	)
	b.ReportAllocs()
	for b.Loop() {
		value, ok = m.Get(i & benchmarkMask)
		i++
	}
	benchmarkValue, benchmarkOK = value, ok
}

func benchmarkWK8Get(b *testing.B) {
	m := newWK8Map()
	var (
		value int
		ok    bool
		i     int
	)
	b.ReportAllocs()
	for b.Loop() {
		value, ok = m.Get(i & benchmarkMask)
		i++
	}
	benchmarkValue, benchmarkOK = value, ok
}

func benchmarkElliottGet(b *testing.B) {
	m := newElliottMap()
	var (
		value int
		ok    bool
		i     int
	)
	b.ReportAllocs()
	for b.Loop() {
		value, ok = m.Get(i & benchmarkMask)
		i++
	}
	benchmarkValue, benchmarkOK = value, ok
}

func benchmarkLorenzoGet(b *testing.B) {
	m := newLorenzoMap()
	var (
		value int
		ok    bool
		i     int
	)
	b.ReportAllocs()
	for b.Loop() {
		value, ok = m.Get(i & benchmarkMask)
		i++
	}
	benchmarkValue, benchmarkOK = value, ok
}

func benchmarkHypermapSetReplace(b *testing.B) {
	m := newHypermap()
	var (
		value int
		ok    bool
		i     int
	)
	b.ReportAllocs()
	for b.Loop() {
		value, ok = m.Replace(i&benchmarkMask, i)
		i++
	}
	benchmarkValue, benchmarkOK = value, ok
}

func benchmarkWK8SetReplace(b *testing.B) {
	m := newWK8Map()
	var (
		value int
		ok    bool
		i     int
	)
	b.ReportAllocs()
	for b.Loop() {
		value, ok = m.Set(i&benchmarkMask, i)
		i++
	}
	benchmarkValue, benchmarkOK = value, ok
}

func benchmarkElliottSetReplace(b *testing.B) {
	m := newElliottMap()
	var (
		ok bool
		i  int
	)
	b.ReportAllocs()
	for b.Loop() {
		ok = m.Set(i&benchmarkMask, i)
		i++
	}
	benchmarkOK = ok
}

func benchmarkLorenzoSetReplace(b *testing.B) {
	m := newLorenzoMap()
	var (
		value int
		err   error
		i     int
	)
	b.ReportAllocs()
	for b.Loop() {
		value, err = m.Update(i&benchmarkMask, i)
		i++
	}
	benchmarkValue, benchmarkErr = value, err
}

func benchmarkHypermapDeleteSet(b *testing.B) {
	m := newHypermap()
	var (
		value int
		ok    bool
		i     int
	)
	b.ReportAllocs()
	for b.Loop() {
		key := i & benchmarkMask
		value, ok = m.Delete(key)
		m.Set(key, i)
		i++
	}
	benchmarkValue, benchmarkOK = value, ok
}

func benchmarkWK8DeleteSet(b *testing.B) {
	m := newWK8Map()
	var (
		value int
		ok    bool
		i     int
	)
	b.ReportAllocs()
	for b.Loop() {
		key := i & benchmarkMask
		value, ok = m.Delete(key)
		m.Set(key, i)
		i++
	}
	benchmarkValue, benchmarkOK = value, ok
}

func benchmarkElliottDeleteSet(b *testing.B) {
	m := newElliottMap()
	var (
		ok bool
		i  int
	)
	b.ReportAllocs()
	for b.Loop() {
		key := i & benchmarkMask
		ok = m.Delete(key)
		m.Set(key, i)
		i++
	}
	benchmarkOK = ok
}

func benchmarkLorenzoDeleteSet(b *testing.B) {
	m := newLorenzoMap()
	var (
		value int
		ok    bool
		err   error
		i     int
	)
	b.ReportAllocs()
	for b.Loop() {
		key := i & benchmarkMask
		value, ok = m.Delete(key)
		err = m.PushBack(key, i)
		i++
	}
	benchmarkValue, benchmarkOK, benchmarkErr = value, ok, err
}

func benchmarkHypermapMoveToFrontBack(b *testing.B) {
	m := newHypermap()
	const key = benchmarkSize / 2
	var ok bool
	b.ReportAllocs()
	for b.Loop() {
		ok = m.MoveToFront(key)
		ok = m.MoveToBack(key) && ok
	}
	benchmarkOK = ok
}

func benchmarkWK8MoveToFrontBack(b *testing.B) {
	m := newWK8Map()
	const key = benchmarkSize / 2
	var err error
	b.ReportAllocs()
	for b.Loop() {
		err = m.MoveToFront(key)
		if err == nil {
			err = m.MoveToBack(key)
		}
	}
	benchmarkErr = err
}

func benchmarkLorenzoMoveToFrontBack(b *testing.B) {
	m := newLorenzoMap()
	const key = benchmarkSize / 2
	var err error
	b.ReportAllocs()
	for b.Loop() {
		err = m.MoveToFront(key)
		if err == nil {
			err = m.MoveToBack(key)
		}
	}
	benchmarkErr = err
}

func benchmarkHypermapRange(b *testing.B) {
	m := newHypermap()
	var sum int
	yield := func(key, value int) bool {
		sum += key + value
		return true
	}
	b.ReportAllocs()
	for b.Loop() {
		m.RangeFunc(yield)
	}
	benchmarkValue = sum
}

func benchmarkWK8Range(b *testing.B) {
	m := newWK8Map()
	var sum int
	b.ReportAllocs()
	for b.Loop() {
		for pair := m.Oldest(); pair != nil; pair = pair.Next() {
			sum += pair.Key + pair.Value
		}
	}
	benchmarkValue = sum
}

func benchmarkElliottRange(b *testing.B) {
	m := newElliottMap()
	var sum int
	yield := func(key, value int) bool {
		sum += key + value
		return true
	}
	b.ReportAllocs()
	for b.Loop() {
		m.AllFromFront()(yield)
	}
	benchmarkValue = sum
}

func benchmarkLorenzoRange(b *testing.B) {
	m := newLorenzoMap()
	var sum int
	yield := func(key, value int) bool {
		sum += key + value
		return true
	}
	b.ReportAllocs()
	for b.Loop() {
		m.Range(yield)
	}
	benchmarkValue = sum
}

func benchmarkHypermapFill(b *testing.B) {
	var length int
	b.ReportAllocs()
	for b.Loop() {
		m := hypermap.New[int, int](benchmarkSize)
		for i := range benchmarkSize {
			m.Set(i, i)
		}
		length = m.Len()
	}
	benchmarkLen = length
}

func benchmarkWK8Fill(b *testing.B) {
	var length int
	b.ReportAllocs()
	for b.Loop() {
		m := wk8.New[int, int](benchmarkSize)
		for i := range benchmarkSize {
			m.Set(i, i)
		}
		length = m.Len()
	}
	benchmarkLen = length
}

func benchmarkElliottFill(b *testing.B) {
	var length int
	b.ReportAllocs()
	for b.Loop() {
		m := elliott.NewOrderedMapWithCapacity[int, int](benchmarkSize)
		for i := range benchmarkSize {
			m.Set(i, i)
		}
		length = m.Len()
	}
	benchmarkLen = length
}

func benchmarkLorenzoFill(b *testing.B) {
	var length int
	b.ReportAllocs()
	for b.Loop() {
		m := lorenzo.New[int, int]()
		for i := range benchmarkSize {
			if err := m.PushBack(i, i); err != nil {
				b.Fatal(err)
			}
		}
		length = m.Len()
	}
	benchmarkLen = length
}

func newHypermap() hypermap.Map[int, int] {
	m := hypermap.New[int, int](benchmarkSize)
	for i := range benchmarkSize {
		m.Set(i, i)
	}
	return m
}

func newWK8Map() *wk8.OrderedMap[int, int] {
	m := wk8.New[int, int](benchmarkSize)
	for i := range benchmarkSize {
		m.Set(i, i)
	}
	return m
}

func newElliottMap() *elliott.OrderedMap[int, int] {
	m := elliott.NewOrderedMapWithCapacity[int, int](benchmarkSize)
	for i := range benchmarkSize {
		m.Set(i, i)
	}
	return m
}

func newLorenzoMap() *lorenzo.OrderedMap[int, int] {
	m := lorenzo.New[int, int]()
	for i := range benchmarkSize {
		if err := m.PushBack(i, i); err != nil {
			panic(err)
		}
	}
	return m
}
