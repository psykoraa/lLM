package main

import (
	"math/rand"
	"sort"
	"strings"
	"testing"
)

// trainNaive je záměrně prostá (pomalá) implementace přesně podle zadání:
// v každém kroku znovu spočítá všechny dvojice, vybere nejlepší (při shodě tu, která se
// v aktuálním rozkladu vyskytne dřív) a sloučí ji. Slouží jako reference pro testy.
func trainNaive(c *Corpus, alg Algorithm, size int) *Result {
	seqs := make([][]string, len(c.Words))
	chars := map[string]bool{}
	for i, w := range c.Words {
		for _, r := range w {
			seqs[i] = append(seqs[i], string(r))
			chars[string(r)] = true
		}
	}
	res := &Result{}
	for s := range chars {
		res.Initial = append(res.Initial, s)
	}
	sort.Strings(res.Initial)
	res.Tokens = append(res.Tokens, res.Initial...)
	inVocab := map[string]bool{}
	for _, s := range res.Initial {
		inVocab[s] = true
	}
	for len(res.Tokens) < size {
		type pr struct{ a, b string }
		counts := map[pr]int{}
		var order []pr
		sym := map[string]int{}
		for i, s := range seqs {
			for j := range s {
				sym[s[j]] += c.Freq[i]
				if j+1 < len(s) {
					k := pr{s[j], s[j+1]}
					if _, ok := counts[k]; !ok {
						order = append(order, k)
					}
					counts[k] += c.Freq[i]
				}
			}
		}
		if len(order) == 0 {
			res.Exhausted = true
			break
		}
		best := order[0]
		for _, k := range order[1:] {
			var better bool
			if alg.usesRatio() {
				// counts[k]/(sym a * sym b) > counts[best]/(...)
				better = counts[k]*sym[best.a]*sym[best.b] > counts[best]*sym[k.a]*sym[k.b]
			} else {
				better = counts[k] > counts[best]
			}
			if better {
				best = k
			}
		}
		m := Merge{A: best.a, B: best.b, New: best.a + best.b, Count: counts[best]}
		if alg.usesRatio() {
			m.CountA, m.CountB = sym[best.a], sym[best.b]
		}
		for i, s := range seqs {
			var nw []string
			for j := 0; j < len(s); j++ {
				if j+1 < len(s) && s[j] == best.a && s[j+1] == best.b {
					nw = append(nw, m.New)
					j++
				} else {
					nw = append(nw, s[j])
				}
			}
			seqs[i] = nw
		}
		m.Dup = inVocab[m.New]
		if !inVocab[m.New] {
			inVocab[m.New] = true
			res.Tokens = append(res.Tokens, m.New)
		}
		res.Merges = append(res.Merges, m)
	}
	return res
}

func randomCorpus(rng *rand.Rand, alphabet string, words, maxLen int) *Corpus {
	b := &corpusBuilder{index: map[string]int{}, c: &Corpus{}}
	for i := 0; i < words; i++ {
		n := 1 + rng.Intn(maxLen)
		seq := make([]rune, n)
		for j := range seq {
			seq[j] = rune(alphabet[rng.Intn(len(alphabet))])
		}
		b.add(seq)
	}
	return b.c
}

func equalResults(t *testing.T, name string, got, want *Result) {
	t.Helper()
	if len(got.Merges) != len(want.Merges) || got.Exhausted != want.Exhausted {
		t.Fatalf("%s: počet sloučení %d (vyčerpáno=%v), očekáváno %d (vyčerpáno=%v)",
			name, len(got.Merges), got.Exhausted, len(want.Merges), want.Exhausted)
	}
	for i := range got.Merges {
		if got.Merges[i] != want.Merges[i] {
			t.Fatalf("%s: sloučení %d: %+v, očekáváno %+v", name, i+1, got.Merges[i], want.Merges[i])
		}
	}
	if strings.Join(got.Tokens, "|") != strings.Join(want.Tokens, "|") {
		t.Fatalf("%s: slovníky se liší", name)
	}
}

// Rychlý algoritmus musí dát přesně totéž co prostá implementace (včetně řešení shod).
func TestMatchesNaive(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	alphabets := []string{"ab", "abc", "abcd", "aabbcde", "abcdefgh"}
	for iter := 0; iter < 300; iter++ {
		alphabet := alphabets[iter%len(alphabets)]
		c := randomCorpus(rng, alphabet, 5+rng.Intn(60), 2+rng.Intn(14))
		for i := range c.Freq { // různé četnosti slov
			c.Freq[i] += rng.Intn(5)
		}
		for _, alg := range []Algorithm{BPE, WordPiece, SentencePiece} {
			size := len(InitialSymbols(c)) + rng.Intn(60)
			got, err := Train(c, alg, size)
			if err != nil {
				t.Fatal(err)
			}
			equalResults(t, alg.String(), got, trainNaive(c, alg, size))
		}
	}
}

// Slovník se zvětšuje až do vyčerpání všech dvojic.
func TestExhaust(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for iter := 0; iter < 50; iter++ {
		c := randomCorpus(rng, "abcd", 30, 8)
		for _, alg := range []Algorithm{BPE, WordPiece} {
			got, _ := Train(c, alg, 1<<30)
			want := trainNaive(c, alg, 1<<30)
			equalResults(t, alg.String(), got, want)
			if !got.Exhausted {
				t.Fatal("slovník měl být vyčerpán")
			}
		}
	}
}

func TestTooSmall(t *testing.T) {
	c, _ := BuildCorpus(strings.NewReader("abc abd"), BPE, false)
	if _, err := Train(c, BPE, 3); err == nil {
		t.Fatal("očekávána chyba: velikost menší než počet znaků")
	}
	if _, err := Train(c, BPE, 4); err != nil {
		t.Fatal(err)
	}
}

func TestCorpusBPE(t *testing.T) {
	c, err := BuildCorpus(strings.NewReader("The the, THE-the! 1905 e=mc2\n"), BPE, false)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for i, w := range c.Words {
		got[string(w)] = c.Freq[i]
	}
	want := map[string]int{"The": 1, "the": 2, "THE": 1, "1905": 1, "e": 1, "mc2": 1}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("slovo %q: %d, očekáváno %d (všechna: %v)", k, got[k], v, got)
		}
	}
	c, _ = BuildCorpus(strings.NewReader("The the THE"), BPE, true)
	if len(c.Words) != 1 || c.Freq[0] != 3 {
		t.Errorf("bez rozlišení velikosti písmen měla být jedna řada ×3, je %v %v", c.Words, c.Freq)
	}
}

func TestCorpusSentencePiece(t *testing.T) {
	c, _ := BuildCorpus(strings.NewReader("  Ahoj   světe, jak se máš?  \n\n  \nAhoj světe, jak se máš?\n"), SentencePiece, false)
	if len(c.Words) != 1 || string(c.Words[0]) != "Ahoj_světe,_jak_se_máš?" || c.Freq[0] != 2 {
		t.Errorf("špatný korpus: %q %v", c.Words, c.Freq)
	}
}

func TestKnownFirstMerges(t *testing.T) {
	c, _ := BuildCorpus(strings.NewReader("aaab aaab aaab ab"), BPE, false)
	res, _ := Train(c, BPE, 100)
	// pár (a,a) je 6× (každé „aaab“ má 2 překrývající se dvojice), (a,b) 4×
	if res.Merges[0].A != "a" || res.Merges[0].B != "a" || res.Merges[0].Count != 6 {
		t.Errorf("první sloučení: %+v", res.Merges[0])
	}
}

func BenchmarkTrain(b *testing.B) {
	rng := rand.New(rand.NewSource(3))
	c := randomCorpus(rng, "abcdefghijklmnopqrstuvwxyz", 20000, 12)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Train(c, BPE, 3000)
	}
}
