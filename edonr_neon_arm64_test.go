//go:build arm64 && !purego

package edonr

import (
	"math/rand"
	"testing"
)

// compress2 must equal the scalar compress on both lanes, over several blocks.
func TestCompress2MatchesScalar(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	var scratch [neonScratch]byte
	for round := 0; round < 50; round++ {
		n := 1 + rng.Intn(5)
		ma, mb := make([]byte, n*BlockSize), make([]byte, n*BlockSize)
		rng.Read(ma)
		rng.Read(mb)
		var sa, sb state
		for j := 0; j < 16; j++ {
			sa[j], sb[j] = rng.Uint64(), rng.Uint64()
		}
		var v state2
		for j := 0; j < 16; j++ {
			v[j][0], v[j][1] = sa[j], sb[j]
		}
		for i := 0; i < n; i++ {
			sa.compress(ma[i*BlockSize:])
			sb.compress(mb[i*BlockSize:])
		}
		compress2(&v, &ma[0], &mb[0], n, &scratch)
		for j := 0; j < 16; j++ {
			if v[j][0] != sa[j] || v[j][1] != sb[j] {
				t.Fatalf("round %d word %d: lanes %x %x, want %x %x", round, j, v[j][0], v[j][1], sa[j], sb[j])
			}
		}
	}
}

func BenchmarkCompress2(b *testing.B) {
	const n = 1024 // 128 KiB per lane
	ma, mb := make([]byte, n*BlockSize), make([]byte, n*BlockSize)
	var v state2
	var scratch [neonScratch]byte
	b.SetBytes(2 * n * BlockSize)
	for i := 0; i < b.N; i++ {
		compress2(&v, &ma[0], &mb[0], n, &scratch)
	}
}
