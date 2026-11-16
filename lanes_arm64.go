//go:build arm64 && !purego

package edonr

import "golang.org/x/sys/cpu"

const maxLanes = 2

// state2 holds the lane states word-major: word j of lane l is at [j*lanes+l].
type state2 [16 * maxLanes]uint64

// lanes is the number of messages hashed at once. lanesOK reports whether the
// multi-buffer kernel can run: it needs the EOR3 and XAR instructions of the
// SHA3 extension.
const lanes = 2

var lanesOK = cpu.ARM64.HasSHA3

type laneScratch [neonScratch]byte

// compressLanes absorbs n blocks of each of the messages at p[0], p[1] into
// the two lanes of s.
func compressLanes(s *state2, p *[maxLanes]*byte, n int, sc *laneScratch) {
	compress2(s, p[0], p[1], n, (*[neonScratch]byte)(sc))
}

// compress2 absorbs n blocks of the message at a and n blocks of the message
// at b into the two lanes of s (edonr_neon_arm64.s, generated: EOR3 and XAR of
// the SHA3 extension). scratch is the staging and spill area.
//
//go:noescape
func compress2(s *state2, a, b *byte, n int, scratch *[neonScratch]byte)
