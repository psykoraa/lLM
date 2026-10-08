package main

import (
	"math/rand"
	"strings"
	"testing"
)

// matrixNaive je doslovný postup ze zadání: pro každou pozici t a každou pozici c ve vzdálenosti
// 1 … Δ před i za ní zvýší M[w_t][c].
func matrixNaive(seq []int32, v, delta int) []uint32 {
	cells := make([]uint32, v*v)
	for t := range seq {
		for d := -delta; d <= delta; d++ {
			u := t + d
			if d == 0 || u < 0 || u >= len(seq) {
				continue
			}
			cells[int(seq[t])*v+int(seq[u])]++
		}
	}
	return cells
}

func TestBuildMatrixMatchesNaive(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for iter := 0; iter < 200; iter++ {
		v := 1 + rng.Intn(8)
		n := rng.Intn(60)
		delta := 1 + rng.Intn(10)
		seq := make([]int32, n)
		for i := range seq {
			seq[i] = int32(rng.Intn(v))
		}
		toks := make([]string, v)
		for i := range toks {
			toks[i] = string(rune('a' + i))
		}
		m, err := BuildMatrix(seq, toks, delta)
		if err != nil {
			t.Fatal(err)
		}
		want := matrixNaive(seq, v, delta)
		for i := range want {
			if m.Cells[i] != want[i] {
				t.Fatalf("v=%d n=%d delta=%d: buňka %d je %d, má být %d", v, n, delta, i, m.Cells[i], want[i])
			}
		}
	}
}

func TestBuildMatrixExample(t *testing.T) {
	// a b a c, Δ = 1: dvojice sousedů ab, ba, ac, každá v obou směrech
	m, err := BuildMatrix([]int32{0, 1, 0, 2}, []string{"a", "b", "c"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := []uint32{
		0, 2, 1,
		2, 0, 0,
		1, 0, 0,
	}
	for i := range want {
		if m.Cells[i] != want[i] {
			t.Fatalf("buňka %d je %d, má být %d", i, m.Cells[i], want[i])
		}
	}
	if m.Total != 6 || m.Nonzero != 4 {
		t.Fatalf("součet %d, nenulových %d", m.Total, m.Nonzero)
	}
}

func TestBuildMatrixErrors(t *testing.T) {
	if _, err := BuildMatrix(nil, []string{"a"}, 0); err == nil {
		t.Fatal("Δ = 0 má selhat")
	}
	if _, err := BuildMatrix(nil, nil, 1); err == nil {
		t.Fatal("prázdný slovník má selhat")
	}
}

// Rozklad textu na tokeny musí tokeny spojit zpět v původní slova a pořadí slov musí zůstat.
func TestTokenSequenceRebuildsText(t *testing.T) {
	text := "the cat sat on the mat. The cat ate the rat; the end of the cat"
	for _, alg := range []Algorithm{BPE, WordPiece, SentencePiece} {
		c, err := BuildCorpusOrdered(strings.NewReader(text), alg, true)
		if err != nil {
			t.Fatal(err)
		}
		res, err := Train(c, alg, 30)
		if err != nil {
			t.Fatal(err)
		}
		seq, err := TokenSequence(c, res)
		if err != nil {
			t.Fatal(err)
		}
		var got, want strings.Builder
		for _, id := range seq {
			got.WriteString(res.Tokens[id])
		}
		for _, w := range c.Order {
			want.WriteString(string(c.Words[w]))
		}
		if got.String() != want.String() {
			t.Fatalf("%v: tokeny dávají %q, má být %q", alg, got.String(), want.String())
		}
		if len(seq) >= want.Len() && len(res.Merges) > 0 {
			t.Fatalf("%v: po sloučeních by mělo být tokenů méně než znaků", alg)
		}
	}
}

func TestPickAndFreq(t *testing.T) {
	m, _ := BuildMatrix([]int32{2, 2, 2, 0, 1}, []string{"a", "b", "c"}, 1)
	if got := m.Pick(1, true); len(got) != 1 || got[0] != 2 {
		t.Fatalf("nejčastější má být c (2), je %v", got)
	}
	if got := m.Pick(2, false); len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Fatalf("první dva podle čísla, je %v", got)
	}
}
