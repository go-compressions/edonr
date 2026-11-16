//go:build !((arm64 || amd64) && !purego)

package edonr

const maxLanes = 1

type state2 [16 * maxLanes]uint64

const lanes = 1

const lanesOK = false

type laneScratch [1]byte

func compressLanes(*state2, *[maxLanes]*byte, int, *laneScratch) {
	panic("edonr: no multi-buffer kernel")
}
