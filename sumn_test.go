package edonr

import (
	"math/rand"
	"testing"
)

// SumN must equal Sum on every message, whatever the mix of lengths per lane
// (padding in one block or two, empty messages, lanes of unequal block counts),
// unkeyed and keyed.
func TestSumNMatchesSum(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	salt := make([]byte, 32)
	rng.Read(salt)
	keyed := NewKeyed(salt)
	special := []int{0, 1, 119, 120, 121, 127, 128, 129, 247, 248, 255, 256, 257, 383, 384, 4096, 16384}
	for round := 0; round < 60; round++ {
		n := 1 + rng.Intn(9)
		msgs := make([][]byte, n)
		for i := range msgs {
			l := rng.Intn(1500)
			if rng.Intn(2) == 0 {
				l = special[rng.Intn(len(special))]
			}
			msgs[i] = make([]byte, l)
			rng.Read(msgs[i])
		}
		for _, d := range []*Digest{New(), keyed} {
			got := make([][Size]byte, n)
			d.SumN(got, msgs)
			for i, m := range msgs {
				c := *d
				c.Reset()
				c.Write(m)
				var want [Size]byte
				c.Sum(want[:0])
				if got[i] != want {
					t.Fatalf("round %d message %d (len %d): SumN differs from Sum", round, i, len(m))
				}
			}
		}
	}
}

func benchSumN(b *testing.B, size, count int) {
	msgs := make([][]byte, count)
	for i := range msgs {
		msgs[i] = make([]byte, size)
	}
	dst := make([][Size]byte, count)
	d := NewKeyed(make([]byte, 32))
	b.SetBytes(int64(size * count))
	for i := 0; i < b.N; i++ {
		d.SumN(dst, msgs)
	}
}

func BenchmarkSumN4K(b *testing.B)   { benchSumN(b, 4096, 64) }
func BenchmarkSumN128K(b *testing.B) { benchSumN(b, 131072, 16) }
