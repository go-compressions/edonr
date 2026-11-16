//go:build amd64 && !purego

package edonr

import (
	"testing"

	"golang.org/x/sys/cpu"
)

// The other width of the multi-buffer kernel (AVX2 when AVX-512 is the default)
// is tested by selecting it explicitly.
func TestLaneWidths(t *testing.T) {
	saved, savedOK := lanes, lanesOK
	defer func() { lanes, lanesOK = saved, savedOK }()
	for _, w := range []int{4, 8} {
		if w == 4 && !cpu.X86.HasAVX2 || w == 8 && !cpu.X86.HasAVX512F {
			continue
		}
		lanes, lanesOK = w, true
		t.Run(map[int]string{4: "avx2", 8: "avx512"}[w], func(t *testing.T) {
			TestCompressLanesMatchesScalar(t)
			TestSumNMatchesSum(t)
		})
	}
}

func BenchmarkLaneWidths(b *testing.B) {
	saved, savedOK := lanes, lanesOK
	defer func() { lanes, lanesOK = saved, savedOK }()
	for _, w := range []int{4, 8} {
		if w == 4 && !cpu.X86.HasAVX2 || w == 8 && !cpu.X86.HasAVX512F {
			continue
		}
		lanes, lanesOK = w, true
		b.Run(map[int]string{4: "avx2", 8: "avx512"}[w], BenchmarkCompressLanes)
	}
}
