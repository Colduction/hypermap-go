package hypermap

import (
	"hash/maphash"
	"math/bits"
)

const (
	indexGroupSlots   = 8
	indexMaxGroupLoad = 7
	indexControlLSB   = uint64(0x0101010101010101)
	indexControlMSB   = uint64(0x8080808080808080)
	indexEmptyControl = uint64(0x80)
	indexDeleted      = uint64(0xfe)
	indexEmptyGroup   = indexControlLSB * indexEmptyControl
	indexControlMask  = uint64(0xff)
	indexMaxGroups    = uint64(1) << 29
	indexMaxCapacity  = indexMaxGroups * indexMaxGroupLoad
)

// indexGroup keeps control bytes beside compact arena references. A control
// byte below 0x80 is the key's seven-bit hash tag; 0x80 is empty and 0xfe is a
// tombstone.
type indexGroup struct {
	control uint64
	refs    [indexGroupSlots]uint32
}

// hashIndex maps keys to stable, non-zero arena references. Group count is a
// power of two and filled, which includes tombstones, never exceeds seven
// eighths of available slots.
type hashIndex[T comparable, T2 any] struct {
	seed   maphash.Seed
	groups []indexGroup
	used   uint32
	filled uint32
}

func makeHashIndex[T comparable, T2 any](capacity int) hashIndex[T, T2] {
	count := indexGroupsForCapacity(capacity)
	groups := make([]indexGroup, count)
	for i := range groups {
		groups[i].control = indexEmptyGroup
	}
	return hashIndex[T, T2]{seed: maphash.MakeSeed(), groups: groups}
}

// get returns key's arena reference, or zero when key is absent.
func (index *hashIndex[T, T2]) get(entries []entries[T, T2], key T) uint32 {
	if len(index.groups) == 0 {
		return 0
	}
	hash := maphash.Comparable(index.seed, key)
	mask := len(index.groups) - 1
	groupIndex := int(hash>>7) & mask
	tag := hash & 0x7f
	var step int
	for {
		group := &index.groups[groupIndex]
		matches := indexMatchTag(group.control, tag)
		for matches != 0 {
			slot := indexFirstMatch(matches)
			ref := group.refs[slot]
			if entries[ref].key == key {
				return ref
			}
			matches &= matches - 1
		}
		if indexMatchEmpty(group.control) != 0 {
			return 0
		}
		step++
		groupIndex = (groupIndex + step) & mask
	}
}

// entry returns key's arena entry, or nil when key is absent.
func (index *hashIndex[T, T2]) entry(entries []entries[T, T2], key T) *entries[T, T2] {
	if len(index.groups) == 0 {
		return nil
	}
	hash := maphash.Comparable(index.seed, key)
	mask := len(index.groups) - 1
	groupIndex := int(hash>>7) & mask
	tag := hash & 0x7f
	var step int
	for {
		group := &index.groups[groupIndex]
		matches := indexMatchTag(group.control, tag)
		for matches != 0 {
			slot := indexFirstMatch(matches)
			entry := &entries[group.refs[slot]]
			if entry.key == key {
				return entry
			}
			matches &= matches - 1
		}
		if indexMatchEmpty(group.control) != 0 {
			return nil
		}
		step++
		groupIndex = (groupIndex + step) & mask
	}
}

// lookup returns key's arena reference and exact table slot.
func (index *hashIndex[T, T2]) lookup(entries []entries[T, T2], key T) (uint32, uint32, bool) {
	if len(index.groups) == 0 {
		return 0, 0, false
	}
	hash := maphash.Comparable(index.seed, key)
	mask := len(index.groups) - 1
	groupIndex := int(hash>>7) & mask
	tag := hash & 0x7f
	var step int
	for {
		group := &index.groups[groupIndex]
		matches := indexMatchTag(group.control, tag)
		for matches != 0 {
			slot := indexFirstMatch(matches)
			ref := group.refs[slot]
			if entries[ref].key == key {
				return ref, indexMakeSlot(groupIndex, slot), true
			}
			matches &= matches - 1
		}
		if indexMatchEmpty(group.control) != 0 {
			return 0, 0, false
		}
		step++
		groupIndex = (groupIndex + step) & mask
	}
}

// findInsert returns an existing reference or the first reusable slot in key's
// probe sequence. index must contain at least one group.
func (index *hashIndex[T, T2]) findInsert(entries []entries[T, T2], key T) (uint32, uint32, uint8) {
	hash := maphash.Comparable(index.seed, key)
	mask := len(index.groups) - 1
	groupIndex := int(hash>>7) & mask
	tag := hash & 0x7f
	var (
		step        int
		deletedSlot uint32
		hasDeleted  bool
	)
	for {
		group := &index.groups[groupIndex]
		matches := indexMatchTag(group.control, tag)
		for matches != 0 {
			slot := indexFirstMatch(matches)
			ref := group.refs[slot]
			if entries[ref].key == key {
				return ref, indexMakeSlot(groupIndex, slot), uint8(tag)
			}
			matches &= matches - 1
		}
		empty := indexMatchEmpty(group.control)
		deleted := indexMatchAvailable(group.control) &^ empty
		if !hasDeleted && deleted != 0 {
			deletedSlot = indexMakeSlot(groupIndex, indexFirstMatch(deleted))
			hasDeleted = true
		}
		if empty != 0 {
			if hasDeleted {
				return 0, deletedSlot, uint8(tag)
			}
			return 0, indexMakeSlot(groupIndex, indexFirstMatch(empty)), uint8(tag)
		}
		step++
		groupIndex = (groupIndex + step) & mask
	}
}

func (index *hashIndex[T, T2]) insert(slot uint32, tag uint8, ref uint32) {
	groupIndex, groupSlot := indexSplitSlot(slot)
	group := &index.groups[groupIndex]
	if indexControlAt(group.control, groupSlot) == indexEmptyControl {
		index.filled++
	}
	indexSetControl(&group.control, groupSlot, uint64(tag))
	group.refs[groupSlot] = ref
	index.used++
}

func (index *hashIndex[T, T2]) deleteSlot(slot uint32) {
	groupIndex, groupSlot := indexSplitSlot(slot)
	group := &index.groups[groupIndex]
	group.refs[groupSlot] = 0
	index.used--
	if indexMatchEmpty(group.control) != 0 {
		indexSetControl(&group.control, groupSlot, indexEmptyControl)
		index.filled--
		return
	}
	indexSetControl(&group.control, groupSlot, indexDeleted)
}

// deleteRef removes the exact arena reference. Reflexive keys use their normal
// probe; non-reflexive keys such as NaN require a reference scan because they
// cannot be found again by equality.
func (index *hashIndex[T, T2]) deleteRef(entries []entries[T, T2], key T, ref uint32) {
	if found, slot, ok := index.lookup(entries, key); ok && found == ref {
		index.deleteSlot(slot)
		return
	}
	if key == key {
		panic("hypermap: index invariant violated")
	}
	for groupIndex := range index.groups {
		group := &index.groups[groupIndex]
		occupied := ^group.control & indexControlMSB
		for occupied != 0 {
			slot := indexFirstMatch(occupied)
			if group.refs[slot] == ref {
				index.deleteSlot(indexMakeSlot(groupIndex, slot))
				return
			}
			occupied &= occupied - 1
		}
	}
	panic("hypermap: index invariant violated")
}

func (index *hashIndex[T, T2]) clear() {
	for i := range index.groups {
		index.groups[i] = indexGroup{control: indexEmptyGroup}
	}
	index.used = 0
	index.filled = 0
}

func (index *hashIndex[T, T2]) maxLoad() uint64 {
	return uint64(len(index.groups)) * indexMaxGroupLoad
}

// growForInsert reports whether an exhausted fill budget should grow instead
// of compacting tombstones. Requiring tombstones in at least one tenth of all
// slots amortizes each same-size rebuild across a proportional number of
// deletions.
func (index *hashIndex[T, T2]) growForInsert() bool {
	maxLoad := index.maxLoad()
	if uint64(index.used) >= maxLoad {
		return true
	}
	if uint64(len(index.groups)) >= indexMaxGroups {
		return false
	}
	tombstones := uint64(index.filled) - uint64(index.used)
	physicalSlots := uint64(len(index.groups)) * indexGroupSlots
	return tombstones*10 < physicalSlots
}

// rebuild removes tombstones at the current size or doubles group storage when
// grow is true.
func (index *hashIndex[T, T2]) rebuild(entries []entries[T, T2], grow bool) {
	count := len(index.groups)
	if grow {
		if uint64(count)*2 > indexMaxGroups {
			panic("hypermap: ordered map capacity too large")
		}
		count *= 2
		index.groups = make([]indexGroup, count)
	} else {
		clear(index.groups)
	}
	for i := range index.groups {
		index.groups[i].control = indexEmptyGroup
	}
	index.used = 0
	index.filled = 0
	for ref := entries[0].next; ref != 0; ref = entries[ref].next {
		index.insertRehashed(entries[ref].key, ref)
	}
}

func (index *hashIndex[T, T2]) insertRehashed(key T, ref uint32) {
	hash := maphash.Comparable(index.seed, key)
	mask := len(index.groups) - 1
	groupIndex := int(hash>>7) & mask
	tag := hash & 0x7f
	var step int
	for {
		group := &index.groups[groupIndex]
		if empty := indexMatchEmpty(group.control); empty != 0 {
			slot := indexFirstMatch(empty)
			indexSetControl(&group.control, slot, tag)
			group.refs[slot] = ref
			index.used++
			index.filled++
			return
		}
		step++
		groupIndex = (groupIndex + step) & mask
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
	return control &^ (control << 6) & indexControlMSB
}

func indexMatchAvailable(control uint64) uint64 {
	return control & indexControlMSB
}

func indexFirstMatch(matches uint64) int {
	return bits.TrailingZeros64(matches) >> 3
}

func indexSetControl(control *uint64, slot int, tag uint64) {
	shift := uint(slot * 8)
	*control = *control&^(indexControlMask<<shift) | tag<<shift
}

func indexControlAt(control uint64, slot int) uint64 {
	return control >> uint(slot*8) & indexControlMask
}

func indexMakeSlot(group, slot int) uint32 {
	return uint32(group)<<3 | uint32(slot)
}

func indexSplitSlot(slot uint32) (int, int) {
	return int(slot >> 3), int(slot & (indexGroupSlots - 1))
}
