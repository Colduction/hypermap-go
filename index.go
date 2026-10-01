package hypermap

import (
	"encoding/binary"
	"hash/maphash"
	"math"
	"math/bits"
	"math/rand/v2"
)

const (
	indexGroupSlots    = 7
	indexMaxGroupLoad  = 6
	indexControlLSB    = uint64(0x0001010101010101)
	indexControlMSB    = uint64(0x0080808080808080)
	indexFullControl   = 0x80
	indexOverflowByte  = 7
	indexOverflowShift = indexOverflowByte * 8
	indexOverflowMax   = 0xff
	indexMaxGroups     = uint64(1) << 29
	indexMaxCapacity   = indexMaxGroups * indexMaxGroupLoad
	indexFreeRef       = ^uint32(0)
	mixPrime           = 0x1d8e4e27c47d124f
)

var hashKey = [...]uint64{rand.Uint64(), rand.Uint64(), rand.Uint64()}

type indexGroup struct {
	control [8]byte
	refs    [indexGroupSlots]uint32
}

type hashIndex[T comparable, T2 any] struct {
	groups  []indexGroup
	seed    maphash.Seed
	mixSeed uint64
	used    uint32
	maxUsed uint32
}

func makeHashIndex[T comparable, T2 any](capacity int) hashIndex[T, T2] {
	count := indexGroupsForCapacity(capacity)
	return hashIndex[T, T2]{
		groups:  make([]indexGroup, count),
		seed:    maphash.MakeSeed(),
		mixSeed: rand.Uint64(),
		maxUsed: uint32(count * indexMaxGroupLoad),
	}
}

func (index *hashIndex[T, T2]) hash(key T) uint64 {
	var x uint64
	switch k := any(key).(type) {
	case int:
		x = uint64(k)
	case int64:
		x = uint64(k)
	case int32:
		x = uint64(k)
	case int16:
		x = uint64(k)
	case int8:
		x = uint64(k)
	case uint:
		x = uint64(k)
	case uint64:
		x = k
	case uint32:
		x = uint64(k)
	case uint16:
		x = uint64(k)
	case uint8:
		x = uint64(k)
	case uintptr:
		x = uint64(k)
	case float64:
		if k == 0 {
			k = 0
		}
		x = math.Float64bits(k)
	case float32:
		if k == 0 {
			k = 0
		}
		x = uint64(math.Float32bits(k))
	case string:
		return hashString(k, index.mixSeed)
	default:
		return maphash.Comparable(index.seed, key)
	}
	return hashInt(x, index.mixSeed)
}

func hashInt(x, seed uint64) uint64 {
	return mix(mixPrime^8, mix(x^hashKey[0], x^seed))
}

func hashString(s string, seed uint64) uint64 {
	var a, b uint64
	if n := len(s); n <= 16 {
		switch {
		case n >= 8:
			a = load64(s, 0)
			b = load64(s, n-8)
		case n >= 4:
			a = load32(s, 0)
			b = load32(s, n-4)
		case n > 0:
			a = uint64(s[0]) | uint64(s[n>>1])<<8 | uint64(s[n-1])<<16
		default:
			return seed
		}
	} else {
		p := s
		if len(p) > 48 {
			seed1, seed2 := seed, seed
			for ; len(p) > 48; p = p[48:] {
				seed = mix(load64(p, 0)^hashKey[0], load64(p, 8)^seed)
				seed1 = mix(load64(p, 16)^hashKey[1], load64(p, 24)^seed1)
				seed2 = mix(load64(p, 32)^hashKey[2], load64(p, 40)^seed2)
			}
			seed ^= seed1 ^ seed2
		}
		for ; len(p) > 16; p = p[16:] {
			seed = mix(load64(p, 0)^hashKey[0], load64(p, 8)^seed)
		}
		a = load64(s, n-16)
		b = load64(s, n-8)
	}
	return mix(mixPrime^uint64(len(s)), mix(a^hashKey[0], b^seed))
}

func load64(s string, i int) uint64 {
	b := s[i : i+8]
	return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
		uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
}

func load32(s string, i int) uint64 {
	b := s[i : i+4]
	return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24
}

func mix(a, b uint64) uint64 {
	hi, lo := bits.Mul64(a, b)
	return hi ^ lo
}

func (m *Map[T, T2]) find(key T) *entries[T, T2] {
	index := &m.index
	if len(index.groups) == 0 {
		return nil
	}
	var hash uint64
	switch k := any(key).(type) {
	case int:
		hash = hashInt(uint64(k), index.mixSeed)
	case string:
		hash = hashString(k, index.mixSeed)
	default:
		hash = index.hash(key)
	}
	groups := index.groups
	mask := len(groups) - 1
	groupIndex := int(hash>>7) & mask
	tag := hash&0x7f | indexFullControl
	for step := 1; ; step++ {
		group := &groups[groupIndex]
		control := binary.LittleEndian.Uint64(group.control[:])
		for matches := indexMatchTag(control, tag); matches != 0; matches &= matches - 1 {
			if entry := &m.entries[group.refs[indexFirstMatch(matches)]]; entry.key == key {
				return entry
			}
		}
		if control>>indexOverflowShift == 0 || step == len(groups) {
			return nil
		}
		groupIndex = (groupIndex + step) & mask
	}
}

func (index *hashIndex[T, T2]) insert(hash uint64, ref uint32) {
	groups := index.groups
	mask := len(groups) - 1
	groupIndex := int(hash>>7) & mask
	for step := 1; ; step++ {
		group := &groups[groupIndex]
		control := binary.LittleEndian.Uint64(group.control[:])
		if empty := indexMatchEmpty(control); empty != 0 {
			slot := indexFirstMatch(empty)
			control |= (hash&0x7f | indexFullControl) << (slot * 8)
			binary.LittleEndian.PutUint64(group.control[:], control)
			group.refs[slot] = ref
			index.used++
			return
		}
		if control>>indexOverflowShift != indexOverflowMax {
			binary.LittleEndian.PutUint64(group.control[:], control+1<<indexOverflowShift)
		}
		groupIndex = (groupIndex + step) & mask
	}
}

func (index *hashIndex[T, T2]) remove(hash uint64, step int, group *indexGroup, slot int) {
	control := binary.LittleEndian.Uint64(group.control[:])
	binary.LittleEndian.PutUint64(group.control[:], control&^(0xff<<(slot*8)))
	index.used--
	groups := index.groups
	mask := len(groups) - 1
	groupIndex := int(hash>>7) & mask
	for i := 1; i < step; i++ {
		passed := &groups[groupIndex]
		if control := binary.LittleEndian.Uint64(passed.control[:]); control>>indexOverflowShift != indexOverflowMax {
			binary.LittleEndian.PutUint64(passed.control[:], control-1<<indexOverflowShift)
		}
		groupIndex = (groupIndex + i) & mask
	}
}

func (index *hashIndex[T, T2]) removeRef(key T, ref uint32) {
	var hash uint64
	switch k := any(key).(type) {
	case int:
		hash = hashInt(uint64(k), index.mixSeed)
	case string:
		hash = hashString(k, index.mixSeed)
	default:
		hash = index.hash(key)
	}
	groups := index.groups
	mask := len(groups) - 1
	groupIndex := int(hash>>7) & mask
	tag := hash&0x7f | indexFullControl
	for step := 1; step <= len(groups); step++ {
		group := &groups[groupIndex]
		for matches := indexMatchTag(binary.LittleEndian.Uint64(group.control[:]), tag); matches != 0; matches &= matches - 1 {
			if slot := indexFirstMatch(matches); group.refs[slot] == ref {
				index.remove(hash, step, group, slot)
				return
			}
		}
		groupIndex = (groupIndex + step) & mask
	}
	panic("hypermap: index invariant violated")
}

func (index *hashIndex[T, T2]) clear(entries []entries[T, T2], unindexed bool) {
	if uint64(index.used)*8 < uint64(len(index.groups)) {
		for ref := 1; ref < len(entries) && index.used != 0; ref++ {
			entry := &entries[ref]
			if entry.prev == indexFreeRef || (unindexed && entry.key != entry.key) {
				continue
			}
			index.removeRef(entry.key, uint32(ref))
		}
	} else {
		clear(index.groups)
		index.used = 0
	}
}

func (index *hashIndex[T, T2]) grow(entries []entries[T, T2], unindexed bool) {
	count := uint64(len(index.groups)) * 2
	if count > indexMaxGroups {
		panic("hypermap: ordered map capacity too large")
	}
	index.groups = make([]indexGroup, count)
	index.maxUsed = uint32(count * indexMaxGroupLoad)
	index.used = 0
	for ref := 1; ref < len(entries); ref++ {
		entry := &entries[ref]
		if entry.prev == indexFreeRef || (unindexed && entry.key != entry.key) {
			continue
		}
		var hash uint64
		switch k := any(entry.key).(type) {
		case int:
			hash = hashInt(uint64(k), index.mixSeed)
		case string:
			hash = hashString(k, index.mixSeed)
		default:
			hash = index.hash(entry.key)
		}
		index.insert(hash, uint32(ref))
	}
}

func indexGroupsForCapacity(capacity int) int {
	if capacity <= 0 {
		return 0
	}
	required := (uint64(capacity) + indexMaxGroupLoad - 1) / indexMaxGroupLoad
	count := uint64(1) << bits.Len64(required-1)
	maxInt := uint64(^uint(0) >> 1)
	if count > maxInt || count > indexMaxGroups {
		panic("hypermap: ordered map capacity too large")
	}
	return int(count)
}

func indexMatchTag(control, tag uint64) uint64 {
	value := control ^ indexControlLSB*tag
	return (value - indexControlLSB) &^ value & indexControlMSB
}

func indexMatchEmpty(control uint64) uint64 {
	vacant := ^control
	return vacant & (vacant << 7) & indexControlMSB
}

func indexFirstMatch(matches uint64) int {
	return bits.TrailingZeros64(matches) >> 3
}
