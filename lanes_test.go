package edonr

import (
	"math/rand"
	"testing"
)

// The multi-buffer kernel must equal the scalar compress in every lane, over
// several blocks, from arbitrary states.
func TestCompressLanesMatchesScalar(t *testing.T) {
	if !lanesOK {
		t.Skip("no multi-buffer kernel on this CPU")
	}
	rng := rand.New(rand.NewSource(11))
	var sc laneScratch
	for round := 0; round < 50; round++ {
		n := 1 + rng.Intn(5)
		var msgs [maxLanes][]byte
		var p [maxLanes]*byte
		var st [maxLanes]state
		var v state2
		for l := 0; l < lanes; l++ {
			msgs[l] = make([]byte, n*BlockSize)
			rng.Read(msgs[l])
			p[l] = &msgs[l][0]
			for j := 0; j < 16; j++ {
				st[l][j] = rng.Uint64()
				v[j*lanes+l] = st[l][j]
			}
			for i := 0; i < n; i++ {
				st[l].compress(msgs[l][i*BlockSize:])
			}
		}
		compressLanes(&v, &p, n, &sc)
		for l := 0; l < lanes; l++ {
			for j := 0; j < 16; j++ {
				if v[j*lanes+l] != st[l][j] {
					t.Fatalf("round %d lane %d word %d: %x, want %x", round, l, j, v[j*lanes+l], st[l][j])
				}
			}
		}
	}
}

func BenchmarkCompressLanes(b *testing.B) {
	if !lanesOK {
		b.Skip("no multi-buffer kernel on this CPU")
	}
	const n = 1024 // 128 KiB per lane
	var msgs [maxLanes][]byte
	var p [maxLanes]*byte
	for l := 0; l < lanes; l++ {
		msgs[l] = make([]byte, n*BlockSize)
		p[l] = &msgs[l][0]
	}
	var v state2
	var sc laneScratch
	b.SetBytes(int64(lanes * n * BlockSize))
	for i := 0; i < b.N; i++ {
		compressLanes(&v, &p, n, &sc)
	}
}
