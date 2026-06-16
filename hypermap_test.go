package hypermap

import (
	"net/url"
	"testing"
)

const benchmarkSize = 4096

var (
	benchmarkValue   int
	benchmarkOK      bool
	benchmarkEncoded string
)

func TestMapSetGetAndOrder(t *testing.T) {
	var m Map[string, int]

	if old, ok := m.Set("b", 2); ok || old != 0 {
		t.Fatalf("Set inserted key returned old=%d ok=%v", old, ok)
	}
	m.Set("a", 1)
	m.Set("c", 3)

	if old, ok := m.Set("a", 11); !ok || old != 1 {
		t.Fatalf("Set replaced key returned old=%d ok=%v", old, ok)
	}
	if got, ok := m.Get("a"); !ok || got != 11 {
		t.Fatalf("Get returned value=%d ok=%v", got, ok)
	}

	wantKeys := []string{"b", "a", "c"}
	wantValues := []int{2, 11, 3}
	var i int
	for key, value := range m.Range() {
		if i >= len(wantKeys) {
			t.Fatalf("Range yielded extra key=%q value=%d", key, value)
		}
		if key != wantKeys[i] || value != wantValues[i] {
			t.Fatalf("Range[%d] = %q/%d, want %q/%d", i, key, value, wantKeys[i], wantValues[i])
		}
		i++
	}
	if i != len(wantKeys) {
		t.Fatalf("Range yielded %d entries, want %d", i, len(wantKeys))
	}
}

func TestMapEncode(t *testing.T) {
	m := New[string, []string](4)
	m.Set("b key", []string{"two words", "x+y"})
	m.Set("a", []string{"1"})
	m.Set("empty", nil)
	m.Set("sym", []string{"a&b=c"})
	m.MoveToFront("sym")

	const want = "sym=a%26b%3Dc&b+key=two+words&b+key=x%2By&a=1"
	if got := Encode(&m); got != want {
		t.Fatalf("Encode = %q, want %q", got, want)
	}
}

func TestMapEncodeEmpty(t *testing.T) {
	if got := Encode[string, []string](nil); got != "" {
		t.Fatalf("Encode on nil map = %q, want empty string", got)
	}

	var m Map[string, []string]
	if got := Encode(&m); got != "" {
		t.Fatalf("Encode on empty map = %q, want empty string", got)
	}

	m.Set("empty", []string{})
	if got := Encode(&m); got != "" {
		t.Fatalf("Encode with empty value slice = %q, want empty string", got)
	}

	m.Set("blank", []string{""})
	if got := Encode(&m); got != "blank=" {
		t.Fatalf("Encode with blank value = %q, want blank=", got)
	}
}

func TestMapEncodeMatchesQueryEscape(t *testing.T) {
	for i := 0; i < 256; i++ {
		s := string([]byte{byte(i)})
		m := New[string, []string](1)
		m.Set(s, []string{s})

		want := url.QueryEscape(s) + "=" + url.QueryEscape(s)
		if got := Encode(&m); got != want {
			t.Fatalf("Encode byte %d = %q, want %q", i, got, want)
		}
	}
}

func BenchmarkMapGet(b *testing.B) {
	m := newBenchmarkMap(benchmarkSize)

	var (
		value int
		ok    bool
	)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		value, ok = m.Get(i & (benchmarkSize - 1))
	}

	benchmarkValue = value
	benchmarkOK = ok
}

func BenchmarkMapSetReplace(b *testing.B) {
	m := newBenchmarkMap(benchmarkSize)

	var (
		value int
		ok    bool
	)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		value, ok = m.Set(i&(benchmarkSize-1), i)
	}

	benchmarkValue = value
	benchmarkOK = ok
}

func BenchmarkMapDeleteSetReuse(b *testing.B) {
	m := newBenchmarkMap(benchmarkSize)

	var (
		value int
		ok    bool
	)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := i & (benchmarkSize - 1)
		value, ok = m.Delete(key)
		m.Set(key, i)
	}

	benchmarkValue = value
	benchmarkOK = ok
}

func BenchmarkMapMoveToFrontBack(b *testing.B) {
	m := newBenchmarkMap(benchmarkSize)

	var ok bool
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := i & (benchmarkSize - 1)
		ok = m.MoveToFront(key)
		ok = m.MoveToBack(key) && ok
	}

	benchmarkOK = ok
}

func BenchmarkMapRange(b *testing.B) {
	m := newBenchmarkMap(benchmarkSize)

	var value int
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for key, item := range m.Range() {
			value += key ^ item
		}
	}

	benchmarkValue = value
}

func BenchmarkMapEncode(b *testing.B) {
	m := New[string, []string](8)
	m.Set("alpha", []string{"1", "2"})
	m.Set("with space", []string{"two words", "x+y"})
	m.Set("symbol", []string{"a&b=c"})
	m.Set("empty", nil)
	m.Set("bytes", []string{"raw-\xff"})
	m.Set("tail", []string{""})

	var encoded string
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encoded = Encode(&m)
	}

	benchmarkEncoded = encoded
}

func newBenchmarkMap(size int) Map[int, int] {
	m := New[int, int](size)
	for i := 0; i < size; i++ {
		m.Set(i, i)
	}
	return m
}

func TestMapDeleteReusesSlot(t *testing.T) {
	m := New[int, int](3)
	m.Set(1, 10)
	m.Set(2, 20)
	m.Set(3, 30)

	if got, ok := m.Delete(2); !ok || got != 20 {
		t.Fatalf("Delete returned value=%d ok=%v", got, ok)
	}
	if m.Has(2) {
		t.Fatal("deleted key is present")
	}

	m.Set(4, 40)
	if m.Len() != 3 {
		t.Fatalf("Len = %d, want 3", m.Len())
	}
	if m.Cap() != 3 {
		t.Fatalf("Cap = %d, want 3", m.Cap())
	}

	want := []int{1, 3, 4}
	var i int
	for key, value := range m.Range() {
		if i >= len(want) {
			t.Fatalf("Range yielded extra key=%d value=%d", key, value)
		}
		if key != want[i] {
			t.Fatalf("Range[%d] key = %d, want %d", i, key, want[i])
		}
		i++
	}
	if i != len(want) {
		t.Fatalf("Range yielded %d entries, want %d", i, len(want))
	}
}

func TestMapMoveAndPop(t *testing.T) {
	m := New[int, string](4)
	m.Set(1, "a")
	m.Set(2, "b")
	m.Set(3, "c")
	m.Set(4, "d")

	if !m.MoveToFront(3) {
		t.Fatal("MoveToFront returned false")
	}
	if !m.MoveToBack(1) {
		t.Fatal("MoveToBack returned false")
	}

	if key, value, ok := m.Front(); !ok || key != 3 || value != "c" {
		t.Fatalf("Front = %d/%q/%v, want 3/c/true", key, value, ok)
	}
	if key, value, ok := m.Back(); !ok || key != 1 || value != "a" {
		t.Fatalf("Back = %d/%q/%v, want 1/a/true", key, value, ok)
	}
	if key, value, ok := m.Next(3); !ok || key != 2 || value != "b" {
		t.Fatalf("Next = %d/%q/%v, want 2/b/true", key, value, ok)
	}
	if key, value, ok := m.Prev(1); !ok || key != 4 || value != "d" {
		t.Fatalf("Prev = %d/%q/%v, want 4/d/true", key, value, ok)
	}

	if key, value, ok := m.PopFront(); !ok || key != 3 || value != "c" {
		t.Fatalf("PopFront = %d/%q/%v, want 3/c/true", key, value, ok)
	}
	if key, value, ok := m.PopBack(); !ok || key != 1 || value != "a" {
		t.Fatalf("PopBack = %d/%q/%v, want 1/a/true", key, value, ok)
	}
	if m.Len() != 2 {
		t.Fatalf("Len = %d, want 2", m.Len())
	}
}

func TestMapClearAndReset(t *testing.T) {
	m := New[int, int](2)
	m.Set(1, 1)
	m.Set(2, 2)
	m.Clear()

	if m.Len() != 0 {
		t.Fatalf("Len after Clear = %d, want 0", m.Len())
	}
	if m.Cap() != 2 {
		t.Fatalf("Cap after Clear = %d, want 2", m.Cap())
	}
	if _, _, ok := m.Front(); ok {
		t.Fatal("Front after Clear returned ok")
	}

	m.Set(3, 3)
	if got, ok := m.Get(3); !ok || got != 3 {
		t.Fatalf("Get after Clear returned value=%d ok=%v", got, ok)
	}

	m.Reset()
	if m.Len() != 0 {
		t.Fatalf("Len after Reset = %d, want 0", m.Len())
	}
	if m.Cap() != 0 {
		t.Fatalf("Cap after Reset = %d, want 0", m.Cap())
	}
}
