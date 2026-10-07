package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

const usage = `Použití:
  llm train [volby] soubor.txt

Sestaví slovník tokenů (BPE, WordPiece nebo SentencePiece) z textového souboru (UTF-8).
Místo souboru lze zadat "-" (čte se standardní vstup).

Volby:
`

func main() {
	args := os.Args[1:]
	switch {
	case len(args) == 0:
		// dvojklik na program: interaktivní režim
		os.Exit(runInteractive(""))
	case args[0] == "train":
		if err := runTrain(args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, "Chyba:", err)
			os.Exit(1)
		}
	case len(args) == 1 && !strings.HasPrefix(args[0], "-") && args[0] != "help":
		// soubor přetažený myší na program: interaktivní režim s tímto souborem
		os.Exit(runInteractive(args[0]))
	default:
		fmt.Fprint(os.Stderr, usage)
		trainFlags().PrintDefaults()
		os.Exit(2)
	}
}

type trainOpts struct {
	algo       string
	size       int
	ignoreCase bool
	out        string
	merges     string
}

var opts trainOpts

func trainFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("train", flag.ContinueOnError)
	fs.StringVar(&opts.algo, "algo", "bpe", "algoritmus: bpe, wordpiece nebo sentencepiece")
	fs.IntVar(&opts.size, "size", 1000, "požadovaná velikost slovníku (počítá se i počáteční slovník jednotlivých znaků)")
	fs.BoolVar(&opts.ignoreCase, "ignore-case", false, "nerozlišovat velká a malá písmena")
	fs.StringVar(&opts.out, "o", "slovnik.tsv", "výstupní soubor se slovníkem (číslo tokenu, tabulátor, token)")
	fs.StringVar(&opts.merges, "merges", "", "volitelně: soubor s postupem sloučení (krok, dvojice, nový symbol, četnosti)")
	return fs
}

func runTrain(args []string) error {
	fs := trainFlags()
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("zadejte právě jeden vstupní soubor (nebo \"-\" pro standardní vstup)")
	}
	alg, ok := ParseAlgorithm(opts.algo)
	if !ok {
		return fmt.Errorf("neznámý algoritmus %q (použijte bpe, wordpiece nebo sentencepiece)", opts.algo)
	}
	return doTrain(fs.Arg(0), alg, opts.size, opts.ignoreCase, opts.out, opts.merges)
}

// doTrain načte text, sestaví slovník a zapíše výsledné soubory (merges může být prázdné).
func doTrain(path string, alg Algorithm, size int, ignoreCase bool, out, merges string) error {
	if size < 1 {
		return fmt.Errorf("velikost slovníku musí být kladné číslo")
	}
	var in io.Reader = os.Stdin
	if path != "-" {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		in = f
	}

	t0 := time.Now()
	corpus, err := BuildCorpus(in, alg, ignoreCase)
	if err != nil {
		return err
	}
	if len(corpus.Words) == 0 {
		return fmt.Errorf("ve vstupu není žádný text")
	}
	tRead := time.Since(t0)

	t1 := time.Now()
	res, err := Train(corpus, alg, size)
	if err != nil {
		if e, ok := err.(ErrTooSmall); ok {
			return fmt.Errorf("slovník jednotlivých znaků má už %d symbolů, zadejte alespoň toto číslo", e.Chars)
		}
		return err
	}
	tTrain := time.Since(t1)

	if err := writeVocab(out, res); err != nil {
		return err
	}
	if merges != "" {
		if err := writeMerges(merges, res, alg.usesRatio()); err != nil {
			return err
		}
	}

	unit := "různých slov"
	if alg == SentencePiece {
		unit = "různých řádků"
	}
	fmt.Fprintf(os.Stderr, "Algoritmus: %s\n", alg)
	fmt.Fprintf(os.Stderr, "Korpus: %d %s (%d celkem)\n", len(corpus.Words), unit, corpus.Total)
	fmt.Fprintf(os.Stderr, "Slovník: %d tokenů (%d jednotlivých znaků a %d sloučení)\n", len(res.Tokens), len(res.Initial), len(res.Merges))
	if res.Exhausted {
		fmt.Fprintf(os.Stderr, "Slovník nelze zvětšit na %d: všechny řady znaků jsou už jediným symbolem.\n", size)
	}
	fmt.Fprintf(os.Stderr, "Čas: načtení %s, trénink %s\n", tRead.Round(time.Millisecond), tTrain.Round(time.Millisecond))
	fmt.Fprintf(os.Stderr, "Zapsáno: %s\n", out)
	if merges != "" {
		fmt.Fprintf(os.Stderr, "Zapsáno: %s\n", merges)
	}
	return nil
}

func writeVocab(path string, res *Result) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	for i, t := range res.Tokens {
		fmt.Fprintf(w, "%d\t%s\n", i+1, t) // přirozená čísla od 1
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// ratioString: poměr #(AB) / (#(A) · #(B)) s desetinnou čárkou.
func ratioString(m Merge) string {
	r := float64(m.Count) / (float64(m.CountA) * float64(m.CountB))
	return strings.Replace(strconv.FormatFloat(r, 'g', 6, 64), ".", ",", 1)
}

func writeMerges(path string, res *Result, ratio bool) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	if ratio {
		fmt.Fprintln(w, "krok\tdvojice\tnový symbol\t#(AB)\t#(A)\t#(B)\tpoměr")
	} else {
		fmt.Fprintln(w, "krok\tdvojice\tnový symbol\tčetnost")
	}
	for i, m := range res.Merges {
		if ratio {
			fmt.Fprintf(w, "%d\t%s + %s\t%s\t%d\t%d\t%d\t%s\n", i+1, m.A, m.B, m.New, m.Count, m.CountA, m.CountB, ratioString(m))
		} else {
			fmt.Fprintf(w, "%d\t%s + %s\t%s\t%d\n", i+1, m.A, m.B, m.New, m.Count)
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
