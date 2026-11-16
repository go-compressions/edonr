//go:build arm64 && !purego

package edonr

// state2 is two double pipes side by side, word-major and lane-minor: word i
// of message 0 is [i][0], of message 1 [i][1].
type state2 [16][2]uint64

// compress2 absorbs n blocks of the message at a and n blocks of the message
// at b into the two lanes of s (edonr_neon_arm64.s, generated: EOR3 and XAR of
// the SHA3 extension). scratch is the staging and spill area.
//
//go:noescape
func compress2(s *state2, a, b *byte, n int, scratch *[neonScratch]byte)
