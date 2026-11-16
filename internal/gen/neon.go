package main

import "fmt"

// neon is two messages at a time. The three-input xor is EOR3 and a rotation
// is XAR against a zero register (the SHA3 extension, mandatory from Armv8.4
// and present on every Apple core). V29 is zero, V30 and V31 hold the affine
// constants.
func neon() *isa {
	v := func(r int) string { return fmt.Sprintf("V%d", r) }
	return &isa{
		name: "neon", build: "//go:build arm64 && !purego", nregs: 29, vb: 16, lanes: 2,
		state: "R0", scratch: "R4", fn: "compress2", frame: "$0-40",
		sig: "// func compress2(s *state2, a, b *byte, n int, scratch *[neonScratch]byte)",
		ld:  func(base string, off, r int) string { return fmt.Sprintf("FMOVQ %d(%s), F%d", off, base, r) },
		st:  func(r int, base string, off int) string { return fmt.Sprintf("FMOVQ F%d, %d(%s)", r, off, base) },
		op: func(k kind, d int, s []int, n int) []string {
			switch k {
			case kAdd:
				return []string{fmt.Sprintf("VADD %s.D2, %s.D2, %s.D2", v(s[1]), v(s[0]), v(d))}
			case kAddC:
				return []string{fmt.Sprintf("VADD V%d.D2, %s.D2, %s.D2", 30+n, v(s[0]), v(d))}
			case kXor:
				return []string{fmt.Sprintf("VEOR %s.B16, %s.B16, %s.B16", v(s[1]), v(s[0]), v(d))}
			case kXor3:
				return []string{fmt.Sprintf("VEOR3 %s.B16, %s.B16, %s.B16, %s.B16", v(s[2]), v(s[1]), v(s[0]), v(d))}
			case kRot:
				return []string{fmt.Sprintf("VXAR $%d, V29.D2, %s.D2, %s.D2", 64-n, v(s[0]), v(d))}
			}
			panic("neon op")
		},
		prologue: func() []string {
			return []string{
				"MOVD s+0(FP), R0", "MOVD a+8(FP), R1", "MOVD b+16(FP), R2", "MOVD n+24(FP), R3", "MOVD scratch+32(FP), R4",
				"VEOR V29.B16, V29.B16, V29.B16",
				"MOVD $0xAAAAAAAAAAAAAAAA, R5", "VDUP R5, V30.D2",
				"MOVD $0x5555555555555555, R5", "VDUP R5, V31.D2",
			}
		},
		stage: func() (l []string) {
			// lane 0 from a, lane 1 from b
			for j := 0; j < 8; j++ {
				l = append(l, fmt.Sprintf("FMOVQ %d(R1), F0", 16*j), fmt.Sprintf("FMOVQ %d(R2), F1", 16*j),
					"VZIP1 V1.D2, V0.D2, V2.D2", "VZIP2 V1.D2, V0.D2, V3.D2",
					fmt.Sprintf("FMOVQ F2, %d(R4)", 32*j), fmt.Sprintf("FMOVQ F3, %d(R4)", 32*j+16))
			}
			return
		},
		advance: func() []string {
			return []string{"ADD $128, R1", "ADD $128, R2", "SUBS $1, R3", "BNE loop", "RET"}
		},
	}
}
