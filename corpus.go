package main

import (
	"bufio"
	"io"
	"strings"
	"unicode"
)

// Algorithm určuje, jak se z textu staví korpus a jak se vybírá dvojice ke sloučení.
type Algorithm int

const (
	BPE           Algorithm = iota // nejčastější dvojice
	WordPiece                      // největší poměr #(AB) / (#(A) · #(B)), korpus = slova
	SentencePiece                  // jako WordPiece, ale mezery jsou "_" a tokeny mohou přesahovat přes slova
)

func (a Algorithm) String() string {
	switch a {
	case WordPiece:
		return "wordpiece"
	case SentencePiece:
		return "sentencepiece"
	}
	return "bpe"
}

// ParseAlgorithm převede název z příkazové řádky na Algorithm.
func ParseAlgorithm(s string) (Algorithm, bool) {
	switch strings.ToLower(s) {
	case "bpe":
		return BPE, true
	case "wordpiece", "wp":
		return WordPiece, true
	case "sentencepiece", "sp":
		return SentencePiece, true
	}
	return BPE, false
}

// usesRatio říká, zda se dvojice vybírá podle poměru (WordPiece, SentencePiece).
func (a Algorithm) usesRatio() bool { return a != BPE }

// Corpus obsahuje různé „řady znaků“ (slova, u SentencePiece celé řádky) v pořadí prvního
// výskytu a počet výskytů každé z nich.
type Corpus struct {
	Words [][]rune
	Freq  []int
	Total int // celkový počet řad (s opakováním)
	// Order je pořadí řad v textu (čísla do Words); vyplní se jen při BuildCorpusOrdered.
	Order []int32
}

// isWordRune: znak slova je písmeno nebo číslice.
func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// lowerRune: malé písmeno; „İ“ zůstává beze změny, stejně jako na webové stránce
// (tam se délka řetězce při převodu na malá písmena nesmí změnit).
func lowerRune(r rune) rune {
	if r == 0x130 {
		return r
	}
	return unicode.ToLower(r)
}

type corpusBuilder struct {
	index       map[string]int
	c           *Corpus
	recordOrder bool
}

func (b *corpusBuilder) add(seq []rune) {
	if len(seq) == 0 {
		return
	}
	b.c.Total++
	key := string(seq)
	i, ok := b.index[key]
	if ok {
		b.c.Freq[i]++
	} else {
		i = len(b.c.Words)
		b.index[key] = i
		b.c.Words = append(b.c.Words, append([]rune(nil), seq...))
		b.c.Freq = append(b.c.Freq, 1)
	}
	if b.recordOrder {
		b.c.Order = append(b.c.Order, int32(i))
	}
}

// BuildCorpus přečte text (UTF-8) po řádcích.
//
//   - BPE a WordPiece: slovo je souvislá řada písmen a číslic; vše ostatní slova odděluje.
//   - SentencePiece: každý neprázdný řádek je jedna řada znaků; řada mezer (i tabulátorů)
//     se nahradí jedním „_“, na začátku a na konci řádku se mezery zahodí, interpunkce zůstává.
func BuildCorpus(r io.Reader, alg Algorithm, ignoreCase bool) (*Corpus, error) {
	return buildCorpus(r, alg, ignoreCase, false)
}

// BuildCorpusOrdered je BuildCorpus, který si navíc pamatuje pořadí řad v textu (Corpus.Order);
// potřebuje ho matice společného výskytu.
func BuildCorpusOrdered(r io.Reader, alg Algorithm, ignoreCase bool) (*Corpus, error) {
	return buildCorpus(r, alg, ignoreCase, true)
}

func buildCorpus(r io.Reader, alg Algorithm, ignoreCase, recordOrder bool) (*Corpus, error) {
	b := &corpusBuilder{index: map[string]int{}, c: &Corpus{}, recordOrder: recordOrder}
	br := bufio.NewReaderSize(r, 1<<20)
	seq := make([]rune, 0, 256)
	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			seq = seq[:0]
			if alg == SentencePiece {
				pendingSpace := false
				for _, ch := range line {
					if ignoreCase {
						ch = lowerRune(ch)
					}
					if unicode.IsSpace(ch) {
						pendingSpace = len(seq) > 0
						continue
					}
					if pendingSpace {
						seq = append(seq, '_')
						pendingSpace = false
					}
					seq = append(seq, ch)
				}
				b.add(seq)
			} else {
				for _, ch := range line {
					if ignoreCase {
						ch = lowerRune(ch)
					}
					if isWordRune(ch) {
						seq = append(seq, ch)
					} else {
						b.add(seq)
						seq = seq[:0]
					}
				}
				b.add(seq)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return b.c, nil
}
