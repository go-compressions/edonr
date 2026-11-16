package main

import (
	"math/rand"
	"sort"
)

// cse plans a common-subexpression elimination for several n-ary sums (or
// xors) over the same operands: Paar's greedy algorithm, repeated with random
// tie-breaking and a fixed seed so that the generated file is reproducible.
// Every step is "new := a op b" over two earlier nodes; each set ends as one node.
type step struct{ a, b int }

func plan(nvars int, sets [][]int, seed int64) (steps []step, result []int) {
	rng := rand.New(rand.NewSource(seed))
	cur := make([][]int, len(sets))
	for i, s := range sets {
		cur[i] = append([]int(nil), s...)
	}
	next := nvars
	for {
		cnt := map[[2]int]int{}
		for _, s := range cur {
			for i := 0; i < len(s); i++ {
				for j := i + 1; j < len(s); j++ {
					a, b := s[i], s[j]
					if a > b {
						a, b = b, a
					}
					cnt[[2]int{a, b}]++
				}
			}
		}
		if len(cnt) == 0 {
			break
		}
		best := 0
		var cands [][2]int
		for k, c := range cnt {
			if c > best {
				best, cands = c, nil
			}
			if c == best {
				cands = append(cands, k)
			}
		}
		sort.Slice(cands, func(i, j int) bool {
			if cands[i][0] != cands[j][0] {
				return cands[i][0] < cands[j][0]
			}
			return cands[i][1] < cands[j][1]
		})
		pick := cands[rng.Intn(len(cands))]
		steps = append(steps, step{pick[0], pick[1]})
		for i, s := range cur {
			ha, hb := false, false
			for _, v := range s {
				ha = ha || v == pick[0]
				hb = hb || v == pick[1]
			}
			if ha && hb {
				var n []int
				for _, v := range s {
					if v != pick[0] && v != pick[1] {
						n = append(n, v)
					}
				}
				cur[i] = append(n, next)
			}
		}
		next++
	}
	for _, s := range cur {
		result = append(result, s[0])
	}
	return
}

// bestPlan keeps the cheapest of many randomised plans.
func bestPlan(nvars int, sets [][]int) ([]step, []int) {
	var bs []step
	var br []int
	for seed := int64(0); seed < 3000; seed++ {
		s, r := plan(nvars, sets, seed)
		if bs == nil || len(s) < len(bs) {
			bs, br = s, r
		}
	}
	return bs, br
}
