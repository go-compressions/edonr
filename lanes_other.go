//go:build !(arm64 && !purego)

package edonr

const lanes = 1

var lanesOK = false

type state2 [16][lanes]uint64

type laneScratch [1]byte

func compressLanes(*state2, *[lanes]*byte, int, *laneScratch) { panic("edonr: no multi-buffer kernel") }
