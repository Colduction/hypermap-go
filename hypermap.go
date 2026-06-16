// Package hypermap provides compact, generic insertion-ordered maps for
// allocation-conscious Go services.
package hypermap

import (
	"iter"
	"strings"
)

const queryEscapeHex = "0123456789ABCDEF"

type orderedMap[T comparable, T2 any] struct {
	index            map[T]int
	entries          []entries[T, T2]
	head, tail, free int
}

type entries[T comparable, T2 any] struct {
	key        T
	value      T2
	prev, next int
}

// Map is an insertion-ordered hash map.
//
// The zero value is ready to use. Single-key lookup, insertion, deletion,
// movement, and front/back access are O(1) average. Use [New] or [Map.Init]
// with the maximum live entry count to keep steady-state writes within existing
// storage. Do not copy a Map after first use. Map does not synchronize access;
// callers that share a [Map] across goroutines need external synchronization.
// Use independent [Map] values per shard or worker when write contention is
// expected.
type Map[T comparable, T2 any] orderedMap[T, T2]

// New returns an initialized [Map] with storage reserved for capacity entries.
func New[T comparable, T2 any](capacity int) Map[T, T2] {
	var m Map[T, T2]
	m.Init(capacity)
	return m
}

// Init prepares m with storage reserved for capacity entries.
func (m *Map[T, T2]) Init(capacity int) {
	if capacity < 0 {
		panic("hypermap: negative ordered map capacity")
	}
	var zero Map[T, T2]
	*m = zero
	if capacity == 0 {
		return
	}
	m.index = make(map[T]int, capacity)
	m.entries = make([]entries[T, T2], 0, capacity)
}

// Len reports the number of entries in m.
func (m *Map[T, T2]) Len() int {
	return len(m.index)
}

// Cap reports the number of entries m can store before growing its slot slice.
func (m *Map[T, T2]) Cap() int {
	return cap(m.entries)
}

// Get returns the value for key.
func (m *Map[T, T2]) Get(key T) (T2, bool) {
	ref := m.index[key]
	if ref == 0 {
		var zero T2
		return zero, false
	}
	return m.entries[ref-1].value, true
}

// Has reports whether key is present in m.
func (m *Map[T, T2]) Has(key T) bool {
	return m.index[key] != 0
}

// Set stores value for key and returns the replaced value when key is present.
func (m *Map[T, T2]) Set(key T, value T2) (T2, bool) {
	if ref := m.index[key]; ref != 0 {
		entry := &m.entries[ref-1]
		old := entry.value
		entry.value = value
		return old, true
	}
	if m.index == nil {
		m.index = make(map[T]int, 1)
	}
	ref := m.addSlot(key, value)
	m.index[key] = ref
	m.linkBack(ref)
	var zero T2
	return zero, false
}

// Delete removes key and returns the removed value when key is present.
func (m *Map[T, T2]) Delete(key T) (T2, bool) {
	ref := m.index[key]
	if ref == 0 {
		var zero T2
		return zero, false
	}
	entry := &m.entries[ref-1]
	old := entry.value
	m.deleteRef(ref, entry, key)
	return old, true
}

// Clear removes all entries while keeping allocated storage for reuse.
func (m *Map[T, T2]) Clear() {
	clear(m.index)
	clear(m.entries)
	m.entries = m.entries[:0]
	m.head = 0
	m.tail = 0
	m.free = 0
}

// Reset removes all entries and releases m backing storage.
func (m *Map[T, T2]) Reset() {
	var zero Map[T, T2]
	*m = zero
}

// Front returns the first key and value in insertion order.
func (m *Map[T, T2]) Front() (T, T2, bool) {
	ref := m.head
	if ref == 0 {
		var zeroKey T
		var zeroValue T2
		return zeroKey, zeroValue, false
	}
	entry := &m.entries[ref-1]
	return entry.key, entry.value, true
}

// Back returns the last key and value in insertion order.
func (m *Map[T, T2]) Back() (T, T2, bool) {
	ref := m.tail
	if ref == 0 {
		var zeroKey T
		var zeroValue T2
		return zeroKey, zeroValue, false
	}
	entry := &m.entries[ref-1]
	return entry.key, entry.value, true
}

// Next returns the key and value after key in insertion order.
func (m *Map[T, T2]) Next(key T) (T, T2, bool) {
	ref := m.index[key]
	if ref == 0 {
		var zeroKey T
		var zeroValue T2
		return zeroKey, zeroValue, false
	}
	next := m.entries[ref-1].next
	if next == 0 {
		var zeroKey T
		var zeroValue T2
		return zeroKey, zeroValue, false
	}
	entry := &m.entries[next-1]
	return entry.key, entry.value, true
}

// Prev returns the key and value before key in insertion order.
func (m *Map[T, T2]) Prev(key T) (T, T2, bool) {
	ref := m.index[key]
	if ref == 0 {
		var zeroKey T
		var zeroValue T2
		return zeroKey, zeroValue, false
	}
	prev := m.entries[ref-1].prev
	if prev == 0 {
		var zeroKey T
		var zeroValue T2
		return zeroKey, zeroValue, false
	}
	entry := &m.entries[prev-1]
	return entry.key, entry.value, true
}

// MoveToFront moves key to the front of m when key is present.
func (m *Map[T, T2]) MoveToFront(key T) bool {
	ref := m.index[key]
	if ref == 0 {
		return false
	}
	if ref == m.head {
		return true
	}
	entry := &m.entries[ref-1]
	m.unlink(entry)
	entry.prev = 0
	entry.next = m.head
	m.entries[m.head-1].prev = ref
	m.head = ref
	return true
}

// MoveToBack moves key to the back of m when key is present.
func (m *Map[T, T2]) MoveToBack(key T) bool {
	ref := m.index[key]
	if ref == 0 {
		return false
	}
	if ref == m.tail {
		return true
	}
	entry := &m.entries[ref-1]
	m.unlink(entry)
	entry.prev = m.tail
	entry.next = 0
	m.entries[m.tail-1].next = ref
	m.tail = ref
	return true
}

// PopFront removes and returns the first key and value in insertion order.
func (m *Map[T, T2]) PopFront() (T, T2, bool) {
	ref := m.head
	if ref == 0 {
		var zeroKey T
		var zeroValue T2
		return zeroKey, zeroValue, false
	}
	entry := &m.entries[ref-1]
	key := entry.key
	value := entry.value
	m.deleteRef(ref, entry, key)
	return key, value, true
}

// PopBack removes and returns the last key and value in insertion order.
func (m *Map[T, T2]) PopBack() (T, T2, bool) {
	ref := m.tail
	if ref == 0 {
		var zeroKey T
		var zeroValue T2
		return zeroKey, zeroValue, false
	}
	entry := &m.entries[ref-1]
	key := entry.key
	value := entry.value
	m.deleteRef(ref, entry, key)
	return key, value, true
}

// Range returns an [iter.Seq2] over each key and value in insertion order.
// Range does not define iteration order after yield mutates m.
func (m *Map[T, T2]) Range() iter.Seq2[T, T2] {
	return func(yield func(T, T2) bool) {
		for ref := m.head; ref != 0; {
			entry := &m.entries[ref-1]
			next := entry.next
			if !yield(entry.key, entry.value) {
				return
			}
			ref = next
		}
	}
}

// Encode encodes m as URL query parameters using [Map]'s current key order.
// Values for each key are encoded in slice order. A nil or empty m encodes
// to an empty string.
func Encode[T ~string, T2 ~[]string](m *Map[T, T2]) string {
	if m == nil || m.Len() == 0 {
		return ""
	}
	size := 0
	for ref := m.head; ref != 0; {
		entry := &m.entries[ref-1]
		next := entry.next
		values := entry.value
		if len(values) == 0 {
			ref = next
			continue
		}
		keySize := queryEscapedLen(string(entry.key))
		for _, value := range values {
			if size > 0 {
				size++
			}
			size += keySize + 1 + queryEscapedLen(value)
		}
		ref = next
	}
	if size == 0 {
		return ""
	}
	var sb strings.Builder
	sb.Grow(size)
	for ref := m.head; ref != 0; {
		entry := &m.entries[ref-1]
		next := entry.next
		if len(entry.value) == 0 {
			ref = next
			continue
		}
		for _, value := range entry.value {
			if sb.Len() > 0 {
				sb.WriteByte('&')
			}
			appendQueryEscaped(&sb, string(entry.key))
			sb.WriteByte('=')
			appendQueryEscaped(&sb, value)
		}
		ref = next
	}
	return sb.String()
}

func queryEscapedLen(s string) int {
	n := len(s)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != ' ' && shouldEscapeQuery(c) {
			n += 2
		}
	}
	return n
}

func appendQueryEscaped(sb *strings.Builder, s string) {
	var start int
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !shouldEscapeQuery(c) {
			continue
		}
		if start < i {
			sb.WriteString(s[start:i])
		}
		if c == ' ' {
			sb.WriteByte('+')
		} else {
			sb.WriteByte('%')
			sb.WriteByte(queryEscapeHex[c>>4])
			sb.WriteByte(queryEscapeHex[c&15])
		}
		start = i + 1
	}
	if start < len(s) {
		sb.WriteString(s[start:])
	}
}

func shouldEscapeQuery(c byte) bool {
	switch {
	case 'a' <= c && c <= 'z':
		return false
	case 'A' <= c && c <= 'Z':
		return false
	case '0' <= c && c <= '9':
		return false
	}
	switch c {
	case '-', '_', '.', '~':
		return false
	}
	return true
}

func (m *Map[T, T2]) addSlot(key T, value T2) int {
	if m.free != 0 {
		ref := m.free
		entry := &m.entries[ref-1]
		m.free = entry.next
		entry.key = key
		entry.value = value
		entry.prev = 0
		entry.next = 0
		return ref
	}
	m.entries = append(m.entries, entries[T, T2]{
		key:   key,
		value: value,
	})
	return len(m.entries)
}

func (m *Map[T, T2]) linkBack(ref int) {
	tail := m.tail
	if tail == 0 {
		m.head = ref
		m.tail = ref
		return
	}
	entry := &m.entries[ref-1]
	entry.prev = tail
	m.entries[tail-1].next = ref
	m.tail = ref
}

func (m *Map[T, T2]) unlink(entry *entries[T, T2]) {
	var (
		prev = entry.prev
		next = entry.next
	)
	if prev == 0 {
		m.head = next
	} else {
		m.entries[prev-1].next = next
	}
	if next == 0 {
		m.tail = prev
	} else {
		m.entries[next-1].prev = prev
	}
}

func (m *Map[T, T2]) deleteRef(ref int, entry *entries[T, T2], key T) {
	delete(m.index, key)
	m.unlink(entry)
	var (
		zeroKey   T
		zeroValue T2
	)
	entry.key = zeroKey
	entry.value = zeroValue
	entry.prev = 0
	entry.next = m.free
	m.free = ref
}
