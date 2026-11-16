package edonr

import (
	"encoding/binary"
	"sort"
)

// SumN stores in dst[i] the digest of msgs[i] (dst must be at least as long as
// msgs), starting from the same state as a fresh digest: unkeyed for New,
// keyed for NewKeyed. The messages are independent, so on CPUs with a
// multi-buffer kernel several of them are hashed at once in the lanes of the
// vector registers (two with NEON; the result is the same as hashing each
// message alone). The best case is many messages of similar length, such as
// the blocks of a ZFS pool. d itself is not modified.
func (d *Digest) SumN(dst [][Size]byte, msgs [][]byte) {
	if len(dst) < len(msgs) {
		panic("edonr: SumN: dst shorter than msgs")
	}
	if !lanesOK {
		for i, m := range msgs {
			dst[i] = d.sumOne(m)
		}
		return
	}
	// pair messages of similar length: lanes run in lockstep
	order := make([]int, len(msgs))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool { return len(msgs[order[a]]) < len(msgs[order[b]]) })
	var sc laneScratch
	i := 0
	for ; i+lanes <= len(order); i += lanes {
		d.sumLanes(dst, msgs, order[i:i+lanes], &sc)
	}
	for ; i < len(order); i++ {
		dst[order[i]] = d.sumOne(msgs[order[i]])
	}
}

// SumN is Digest.SumN for the unkeyed hash.
func SumN(dst [][Size]byte, msgs [][]byte) { New().SumN(dst, msgs) }

func (d *Digest) sumOne(m []byte) [Size]byte {
	c := *d
	c.Reset()
	c.Write(m)
	var out [Size]byte
	c.Sum(out[:0])
	return out
}

// segment is a run of whole blocks.
type segment []byte

// sumLanes hashes msgs[idx[l]] in lane l.
func (d *Digest) sumLanes(dst [][Size]byte, msgs [][]byte, idx []int, sc *laneScratch) {
	var v state2
	var st [lanes]state
	var tails [lanes][2 * BlockSize]byte
	var segs [lanes][2]segment
	for l := 0; l < lanes; l++ {
		m := msgs[idx[l]]
		full := len(m) / BlockSize
		rem := m[full*BlockSize:]
		t := &tails[l]
		copy(t[:], rem)
		t[len(rem)] = 0x80
		end := BlockSize
		if len(rem) >= BlockSize-8 {
			end = 2 * BlockSize
		}
		binary.LittleEndian.PutUint64(t[end-8:], (d.n0+uint64(len(m)))<<3)
		segs[l] = [2]segment{m[:full*BlockSize], t[:end]}
		st[l] = d.start
		for j := 0; j < 16; j++ {
			v[j][l] = st[l][j]
		}
	}
	// lockstep while every lane has blocks left
	for {
		n := -1
		var p [lanes]*byte
		for l := 0; l < lanes; l++ {
			for len(segs[l][0]) == 0 && len(segs[l][1]) > 0 {
				segs[l][0], segs[l][1] = segs[l][1], nil
			}
			c := len(segs[l][0]) / BlockSize
			if n < 0 || c < n {
				n = c
			}
			if c > 0 {
				p[l] = &segs[l][0][0]
			}
		}
		if n <= 0 {
			break
		}
		compressLanes(&v, &p, n, sc)
		for l := 0; l < lanes; l++ {
			segs[l][0] = segs[l][0][n*BlockSize:]
		}
	}
	// a lane that still has blocks finishes alone
	for l := 0; l < lanes; l++ {
		for j := 0; j < 16; j++ {
			st[l][j] = v[j][l]
		}
		for _, sg := range segs[l] {
			for ; len(sg) >= BlockSize; sg = sg[BlockSize:] {
				st[l].compress(sg)
			}
		}
		out := &dst[idx[l]]
		for i := 0; i < 8; i++ {
			binary.LittleEndian.PutUint64(out[8*i:], st[l][8+i])
		}
	}
}
