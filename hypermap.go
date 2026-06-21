// Package hypermap provides compact, generic insertion-ordered maps for
// allocation-conscious Go services.
package hypermap

import (
	"iter"
	"strings"
)

const queryEscapeHex = "0123456789ABCDEF"

// orderedMap stores its entries in a slice whose first element (index 0) is a
// permanent sentinel. The sentinel anchors a circular doubly-linked list, so
// entries[0].next is the head, entries[0].prev is the tail, and an empty list
// is the sentinel pointing at itself. Anchoring the list this way removes the
// need for separate head/tail fields and lets link/unlink run without
// boundary checks. Deleted slots are recycled through a singly-linked free
// chain rooted at free, so steady-state insert/delete reuses storage instead
// of growing it.
type orderedMap[T comparable, T2 any] struct {
	index   map[T]int
	entries []entries[T, T2]
	free    int
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

// New returns an initialized [Map] with storage reserved for up to capacity
// live entries.
func New[T comparable, T2 any](capacity int) Map[T, T2] {
	var m Map[T, T2]
	m.Init(capacity)
	return m
}

// Init prepares m with storage reserved for up to capacity live entries.
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
	m.entries = make([]entries[T, T2], 1, capacity+1)
}

// Len reports the number of entries in m.
func (m *Map[T, T2]) Len() int {
	return len(m.index)
}

// Cap reports the maximum live entries m can hold before growing its slot slice.
func (m *Map[T, T2]) Cap() int {
	c := cap(m.entries)
	if c == 0 {
		return 0
	}
	return c - 1
}

// Get returns the value for key and reports whether key is present.
func (m *Map[T, T2]) Get(key T) (T2, bool) {
	ref := m.index[key]
	if ref == 0 {
		var zero T2
		return zero, false
	}
	return m.entries[ref].value, true
}

// Has reports whether key is present in m.
func (m *Map[T, T2]) Has(key T) bool {
	return m.index[key] != 0
}

// Set stores value for key and returns the replaced value when key is present.
func (m *Map[T, T2]) Set(key T, value T2) (T2, bool) {
	if ref := m.index[key]; ref != 0 {
		var (
			entry = &m.entries[ref]
			old   = entry.value
		)
		entry.value = value
		return old, true
	}
	if m.index == nil {
		m.index = make(map[T]int, 1)
	}
	if len(m.entries) == 0 {
		m.entries = append(m.entries, entries[T, T2]{}) // sentinel
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
	old := m.entries[ref].value
	m.deleteRef(ref, &m.entries[ref], key)
	return old, true
}

// Clear removes all entries while keeping allocated storage for reuse.
func (m *Map[T, T2]) Clear() {
	clear(m.index)
	if len(m.entries) != 0 {
		clear(m.entries)
		m.entries = m.entries[:1] // keep the zeroed sentinel
	}
	m.free = 0
}

// Reset removes all entries and releases m backing storage.
func (m *Map[T, T2]) Reset() {
	var zero Map[T, T2]
	*m = zero
}

// Front returns the first key and value in insertion order and reports whether
// m is non-empty.
func (m *Map[T, T2]) Front() (T, T2, bool) {
	if len(m.entries) == 0 {
		var (
			zeroKey   T
			zeroValue T2
		)
		return zeroKey, zeroValue, false
	}
	ref := m.entries[0].next
	if ref == 0 {
		var (
			zeroKey   T
			zeroValue T2
		)
		return zeroKey, zeroValue, false
	}
	entry := &m.entries[ref]
	return entry.key, entry.value, true
}

// Back returns the last key and value in insertion order and reports whether m
// is non-empty.
func (m *Map[T, T2]) Back() (T, T2, bool) {
	if len(m.entries) == 0 {
		var (
			zeroKey   T
			zeroValue T2
		)
		return zeroKey, zeroValue, false
	}
	ref := m.entries[0].prev
	if ref == 0 {
		var (
			zeroKey   T
			zeroValue T2
		)
		return zeroKey, zeroValue, false
	}
	entry := &m.entries[ref]
	return entry.key, entry.value, true
}

// Next returns the key and value after key in insertion order and reports
// whether key has a successor.
func (m *Map[T, T2]) Next(key T) (T, T2, bool) {
	ref := m.index[key]
	if ref == 0 {
		var (
			zeroKey   T
			zeroValue T2
		)
		return zeroKey, zeroValue, false
	}
	next := m.entries[ref].next
	if next == 0 {
		var (
			zeroKey   T
			zeroValue T2
		)
		return zeroKey, zeroValue, false
	}
	entry := &m.entries[next]
	return entry.key, entry.value, true
}

// Prev returns the key and value before key in insertion order and reports
// whether key has a predecessor.
func (m *Map[T, T2]) Prev(key T) (T, T2, bool) {
	ref := m.index[key]
	if ref == 0 {
		var (
			zeroKey   T
			zeroValue T2
		)
		return zeroKey, zeroValue, false
	}
	prev := m.entries[ref].prev
	if prev == 0 {
		var (
			zeroKey   T
			zeroValue T2
		)
		return zeroKey, zeroValue, false
	}
	entry := &m.entries[prev]
	return entry.key, entry.value, true
}

// MoveToFront moves key to the front of m when key is present.
func (m *Map[T, T2]) MoveToFront(key T) bool {
	ref := m.index[key]
	if ref == 0 {
		return false
	}
	var (
		e    = m.entries
		head = e[0].next
	)
	if head == ref {
		return true
	}
	var (
		entry      = &e[ref]
		prev, next = entry.prev, entry.next
	)
	e[prev].next = next
	e[next].prev = prev
	entry.prev = 0
	entry.next = head
	e[head].prev = ref
	e[0].next = ref
	return true
}

// MoveToBack moves key to the back of m when key is present.
func (m *Map[T, T2]) MoveToBack(key T) bool {
	ref := m.index[key]
	if ref == 0 {
		return false
	}
	var (
		e    = m.entries
		tail = e[0].prev
	)
	if tail == ref {
		return true
	}
	var (
		entry      = &e[ref]
		prev, next = entry.prev, entry.next
	)
	e[prev].next = next
	e[next].prev = prev
	entry.next = 0
	entry.prev = tail
	e[tail].next = ref
	e[0].prev = ref
	return true
}

// PopFront removes and returns the first key and value in insertion order and
// reports whether an entry is removed.
func (m *Map[T, T2]) PopFront() (T, T2, bool) {
	if len(m.entries) == 0 {
		var (
			zeroKey   T
			zeroValue T2
		)
		return zeroKey, zeroValue, false
	}
	ref := m.entries[0].next
	if ref == 0 {
		var (
			zeroKey   T
			zeroValue T2
		)
		return zeroKey, zeroValue, false
	}
	var (
		entry      = &m.entries[ref]
		key, value = entry.key, entry.value
	)
	m.deleteRef(ref, entry, key)
	return key, value, true
}

// PopBack removes and returns the last key and value in insertion order and
// reports whether an entry is removed.
func (m *Map[T, T2]) PopBack() (T, T2, bool) {
	if len(m.entries) == 0 {
		var (
			zeroKey   T
			zeroValue T2
		)
		return zeroKey, zeroValue, false
	}
	ref := m.entries[0].prev
	if ref == 0 {
		var (
			zeroKey   T
			zeroValue T2
		)
		return zeroKey, zeroValue, false
	}
	var (
		entry      = &m.entries[ref]
		key, value = entry.key, entry.value
	)
	m.deleteRef(ref, entry, key)
	return key, value, true
}

// Range returns an [iter.Seq2] over each key and value in insertion order.
// Range leaves remaining iteration behavior unspecified when yield mutates m.
func (m *Map[T, T2]) Range() iter.Seq2[T, T2] {
	return func(yield func(T, T2) bool) {
		e := m.entries
		if len(e) == 0 {
			return
		}
		for ref := e[0].next; ref != 0; {
			var (
				entry = &e[ref]
				next  = entry.next
			)
			if !yield(entry.key, entry.value) {
				return
			}
			ref = next
		}
	}
}

// QueryMap is a [Map] specialized for URL query parameters. It maps string keys
// to repeated string values and adds [QueryMap.Encode]. Every [Map] method is
// promoted for ordered string keys and []string values.
type QueryMap struct {
	Map[string, []string]
}

// NewQueryMap returns an initialized [QueryMap] with storage reserved for up to
// capacity live entries.
func NewQueryMap(capacity int) QueryMap {
	return QueryMap{New[string, []string](capacity)}
}

// Encode returns qm encoded as URL query parameters in key insertion order.
// Values for each key are encoded in slice order. A nil or empty [QueryMap]
// encodes to an empty string.
func (qm *QueryMap) Encode() string {
	if qm == nil || qm.Len() == 0 {
		return ""
	}
	var (
		e    = qm.entries
		size int
	)
	for ref := e[0].next; ref != 0; {
		var (
			entry  = &e[ref]
			next   = entry.next
			values = entry.value
		)
		if len(values) == 0 {
			ref = next
			continue
		}
		keySize := queryEscapedLen(entry.key)
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
	for ref := e[0].next; ref != 0; {
		var (
			entry = &e[ref]
			next  = entry.next
		)
		for _, value := range entry.value {
			if sb.Len() > 0 {
				sb.WriteByte('&')
			}
			appendQueryEscaped(&sb, entry.key)
			sb.WriteByte('=')
			appendQueryEscaped(&sb, value)
		}
		ref = next
	}
	return sb.String()
}

// queryEscape[c] reports whether byte c must be percent-escaped in a query
// component. Precomputing it turns the per-byte classification in the encode
// hot loops into a single branch-free load. Indexing a [256]bool with a byte
// needs no bounds check.
var queryEscape = func() (t [256]bool) {
	for c := range len(t) {
		t[c] = shouldEscapeQuery(byte(c))
	}
	return t
}()

func queryEscapedLen(s string) int {
	n := len(s)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != ' ' && queryEscape[c] {
			n += 2
		}
	}
	return n
}

func appendQueryEscaped(sb *strings.Builder, s string) {
	var start int
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !queryEscape[c] {
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

// addSlot reserves a slot for key/value and returns its slice index, reusing a
// freed slot when one is available. The caller links the returned slot into the
// list.
func (m *Map[T, T2]) addSlot(key T, value T2) int {
	if m.free != 0 {
		var (
			ref   = m.free
			entry = &m.entries[ref]
		)
		m.free = entry.next
		entry.key = key
		entry.value = value
		entry.prev = 0
		entry.next = 0
		return ref
	}
	m.entries = append(m.entries, entries[T, T2]{key: key, value: value})
	return len(m.entries) - 1
}

// linkBack appends ref to the tail of the circular list.
func (m *Map[T, T2]) linkBack(ref int) {
	var (
		tail  = m.entries[0].prev
		entry = &m.entries[ref]
	)
	entry.prev = tail
	entry.next = 0
	m.entries[tail].next = ref
	m.entries[0].prev = ref
}

// unlink removes entry from the list. The circular sentinel makes both the
// head and tail cases fall through to the same two writes, so no branching is
// needed.
func (m *Map[T, T2]) unlink(entry *entries[T, T2]) {
	m.entries[entry.prev].next = entry.next
	m.entries[entry.next].prev = entry.prev
}

// deleteRef removes entry (the slot at ref) from the list and pushes its slot
// onto the free chain for reuse by a later insert.
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
