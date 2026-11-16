//go:build arm64 && !purego

package edonr

import "golang.org/x/sys/cpu"

const lanes = 2

// lanesOK reports whether the multi-buffer kernel can run: it needs the
// EOR3 and XAR instructions of the SHA3 extension.
var lanesOK = cpu.ARM64.HasSHA3

type laneScratch [neonScratch]byte

// compressLanes absorbs n blocks of each of the messages at p[0], p[1] into
// the two lanes of s.
func compressLanes(s *state2, p *[lanes]*byte, n int, sc *laneScratch) {
	compress2(s, p[0], p[1], n, (*[neonScratch]byte)(sc))
}
