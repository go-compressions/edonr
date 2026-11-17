package edonr

import (
	"context"
	"encoding/binary"
	"runtime"
	"sort"

	"github.com/go-filesystems/inflight"
)

// SumN stores in dst[i] the digest of msgs[i] (dst must be at least as long as
// msgs), starting from the same state as a fresh digest: unkeyed for New,
// keyed for NewKeyed. The messages are independent, so on CPUs with a
// multi-buffer kernel several of them are hashed at once in the lanes of the
// vector registers (2 with NEON, 4 with AVX2, 8 with AVX-512; the result is the same as hashing each
// message alone). The best case is many messages of similar length, such as
// the blocks of a ZFS pool. d itself is not modified.
func (d *Digest) SumN(dst [][Size]byte, msgs [][]byte) { d.SumNParallel(dst, msgs, 1) }

// SumNParallel is SumN on up to workers goroutines (workers <= 0 means
// GOMAXPROCS): the messages are sorted by length and handed out in groups of
// one lane width, so every goroutine runs the multi-buffer kernel on messages
// of similar length. The digests do not depend on workers.
func (d *Digest) SumNParallel(dst [][Size]byte, msgs [][]byte, workers int) {
	if len(dst) < len(msgs) {
		panic("edonr: SumN: dst shorter than msgs")
	}
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	width := 1
	if lanesOK {
		width = lanes
	}
	// pair messages of similar length: lanes run in lockstep
	order := make([]int, len(msgs))
	for i := range order {
		order[i] = i
	}
	if width > 1 {
		sort.Slice(order, func(a, b int) bool { return len(msgs[order[a]]) < len(msgs[order[b]]) })
	}
	// the incomplete group, if any, is the first one: the shortest messages
	// are the cheapest to hash without the kernel
	r := len(order) % width
	groups := len(order) / width
	if r > 0 {
		groups++
	}
	if workers > groups {
		workers = groups
	}
	bounds := func(g int) (int, int) {
		if r == 0 {
			return g * width, (g + 1) * width
		}
		if g == 0 {
			return 0, r
		}
		return r + (g-1)*width, r + g*width
	}
	run := func(g0, g1 int) {
		var sc laneScratch
		for g := g0; g < g1; g++ {
			lo, hi := bounds(g)
			idx := order[lo:hi]
			if width > 1 && len(idx) == width {
				d.sumLanes(dst, msgs, idx, &sc)
				continue
			}
			for _, i := range idx {
				dst[i] = d.sumOne(msgs[i])
			}
		}
	}
	if workers <= 1 {
		run(0, groups)
		return
	}
	// The scheduling is the fleet's: inflight.Ordered runs the chunks on a
	// bounded set of goroutines. Each chunk writes its own digests into dst, so
	// there is nothing to emit; a chunk is a few groups, to amortise the
	// scheduling and stay balanced when lengths differ.
	chunk := max(1, groups/(workers*8))
	items := (groups + chunk - 1) / chunk
	_ = inflight.Each(context.Background(), items, workers,
		func(_ context.Context, i int) error {
			run(i*chunk, min((i+1)*chunk, groups))
			return nil
		})
}

// SumNParallel is Digest.SumNParallel for the unkeyed hash.
func SumNParallel(dst [][Size]byte, msgs [][]byte, workers int) {
	New().SumNParallel(dst, msgs, workers)
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
	var st [maxLanes]state
	var tails [maxLanes][2 * BlockSize]byte
	var segs [maxLanes][2]segment
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
			v[j*lanes+l] = st[l][j]
		}
	}
	// lockstep while every lane has blocks left
	for {
		n := -1
		var p [maxLanes]*byte
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
			st[l][j] = v[j*lanes+l]
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
