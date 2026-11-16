//go:build amd64 && !purego

package edonr

import "golang.org/x/sys/cpu"

const maxLanes = 8

// state2 holds the lane states word-major: word j of lane l is at
// [j*lanes+l]; it is wide enough for the widest kernel.
type state2 [16 * maxLanes]uint64

// lanes is the number of messages hashed at once: 8 with AVX-512, 4 with AVX2.
// lanesOK reports whether a multi-buffer kernel can run.
var lanes, lanesOK = func() (int, bool) {
	switch {
	case cpu.X86.HasAVX512F:
		return 8, true
	case cpu.X86.HasAVX2:
		return 4, true
	}
	return 1, false
}()

type laneScratch [avx512Scratch]byte

// compressLanes absorbs n blocks of each of the messages at p[0..lanes) into
// the lanes of s.
func compressLanes(s *state2, p *[maxLanes]*byte, n int, sc *laneScratch) {
	if lanes == 8 {
		compress8(s, p, n, (*[avx512Scratch]byte)(sc))
	} else {
		compress4(s, (*[4]*byte)(p[:4]), n, (*[avx2Scratch]byte)(sc[:avx2Scratch]))
	}
}

// func compress4(s *state2, p *[4]*byte, n int, scratch *[avx2Scratch]byte)
//
//go:noescape
func compress4(s *state2, p *[4]*byte, n int, scratch *[avx2Scratch]byte)

// func compress8(s *state2, p *[8]*byte, n int, scratch *[avx512Scratch]byte)
//
//go:noescape
func compress8(s *state2, p *[8]*byte, n int, scratch *[avx512Scratch]byte)
