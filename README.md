# LLM – programy k semináři o velkých jazykových modelech

Program do počítače, který z textového souboru sestaví **slovník tokenů** třemi algoritmy,
které jsou i na webové stránce [matematika](https://github.com/psykoraa/matematika)
(`albert-einstein.html`): **BPE**, **WordPiece** a **SentencePiece**. Dává **stejné výsledky
jako web** (včetně řešení shod), ale je mnohonásobně rychlejší, takže zvládne i velké texty
a slovníky o tisících tokenů během zlomku sekundy.

## Rychlý start

Hotové programy jsou ve složce [`bin/`](bin/) (nic se neinstaluje):

| Počítač | Soubor |
|---|---|
| Windows | `bin/llm-windows-amd64.exe` |
| macOS (Apple Silicon M1 a novější) | `bin/llm-macos-apple-silicon` |
| macOS (Intel) | `bin/llm-macos-intel` |
| Linux | `bin/llm-linux-amd64` |

**Nejjednodušeji (Windows):**

1. Stáhněte `llm-windows-amd64.exe` a ukázkový text `priklady/einstein.txt` do jedné složky.
2. **Dvakrát klikněte na program** (otevře se černé okno), nebo na něj **myší přetáhněte textový soubor**.
3. Program se zeptá na soubor, algoritmus (1 = BPE, 2 = WordPiece, 3 = SentencePiece), velikost slovníku
   a rozlišování velkých a malých písmen. Potom výsledek zapíše vedle vašeho textu
   (`…-slovnik.tsv` a `…-postup.tsv`). Okno se nezavře, dokud nestisknete Enter.

**Z příkazového řádku:**

```
llm-windows-amd64.exe train -algo bpe -size 5000 -o slovnik.tsv einstein.txt
```

> Windows může při prvním spuštění zobrazit upozornění SmartScreen (program není podepsaný);
> zvolte „Další informace → Přesto spustit“. Na macOS a Linuxu je potřeba soubor nejdřív
> povolit ke spuštění: `chmod +x bin/llm-macos-apple-silicon`.

## Použití

```
llm train [volby] soubor.txt
```

Vstupem je textový soubor v kódování **UTF-8** (nebo `-` pro standardní vstup).

| Volba | Význam |
|---|---|
| `-algo` | `bpe` (výchozí), `wordpiece` nebo `sentencepiece` |
| `-size` | požadovaná velikost slovníku; počítá se i počáteční slovník jednotlivých znaků (výchozí 1000) |
| `-ignore-case` | nerozlišovat velká a malá písmena (na webu je to výchozí) |
| `-o` | výstupní soubor se slovníkem (výchozí `slovnik.tsv`) |
| `-merges` | volitelně soubor s postupem všech sloučení |

Příklady (ukázkový text je ve složce [`priklady/`](priklady/)):

```
llm train -algo bpe -size 5000 -ignore-case -o bpe.tsv einstein.txt
llm train -algo wordpiece -size 5000 -ignore-case -o wp.tsv -merges wp-postup.tsv einstein.txt
llm train -algo sentencepiece -size 2000 -o sp.tsv einstein.txt
```

### Výstup

`slovnik.tsv` – na každém řádku číslo tokenu (přirozené číslo od 1), tabulátor a token.
Nejdřív jsou jednotlivé znaky (seřazené), pak sloučené symboly v pořadí vzniku:

```
1	0
2	1
...
75	in
76	th
```

Soubor se `-merges` obsahuje postup: krok, dvojice, nový symbol a četnosti
(u WordPiece a SentencePiece `#(AB)`, `#(A)`, `#(B)` a poměr).

## Jak algoritmy fungují

Všechny tři začínají slovníkem jednotlivých znaků a opakují: spočítat dvojice sousedních
symbolů → vybrat jednu → nahradit ji novým symbolem a přidat ho do slovníku → opakovat,
dokud slovník nemá požadovanou velikost.

| | Korpus | Výběr dvojice (A, B) |
|---|---|---|
| **BPE** | slova (souvislé řady písmen a číslic) | největší `#(AB)` |
| **WordPiece** | slova | největší poměr `#(AB) / (#(A) · #(B))` |
| **SentencePiece** | každý řádek souboru = jedna řada znaků, mezery se nahradí „_“, interpunkce zůstává; tokeny mohou přesahovat přes slova | největší poměr jako u WordPiece |

- `#(AB)` je počet výskytů dvojice vedle sebe (počítají se i překrývající se dvojice), `#(A)`
  a `#(B)` jsou počty výskytů jednotlivých symbolů v **aktuálním** rozkladu textu.
- Při shodě skóre vyhrává dvojice, která se v aktuálním rozkladu textu vyskytne dřív.
- Poměry se porovnávají přesně (celými čísly, bez zaokrouhlení).
- U SentencePiece se řada mezer na řádku nahradí jedním „_“ a mezery na začátku a na konci
  řádku se zahodí.
- Slovo je písmeno nebo číslice (`unicode.IsLetter`, `unicode.IsDigit`), vše ostatní slova odděluje.

## Proč je to rychlé

Webová stránka po každém sloučení znovu spočítá všechny dvojice v celém textu, takže pro
slovník o *M* tokenech prochází text *M*krát. Program místo toho:

1. každé různé slovo uloží jen jednou (s počtem výskytů),
2. po sloučení **opraví jen četnosti dvojic kolem sloučených míst** (ne celý text),
3. drží dvojice v **haldě podle skóre** (vybrat nejlepší je rychlé), zastaralé položky
   se zahazují až při výběru,
4. pro každou dvojici si pamatuje, ve kterých slovech je (stačí upravit jen ta),
5. u WordPiece a SentencePiece po sloučení přepočítá skóre jen dvojic, které obsahují
   dotčené symboly.

Výsledek je **přesně stejný** jako u prosté implementace – testy to ověřují na stovkách
náhodných textů, včetně shod skóre.

Naměřeno (slovník o 5000 tokenech; web = stránka v prohlížeči, program = tento nástroj):

| Text | Algoritmus | Web | Program |
|---|---|---|---|
| Albert Einstein (14 307 slov) | BPE | 9,1 s | 0,04 s |
| Albert Einstein | WordPiece | 16,0 s | 0,3 s |
| Albert Einstein (slovník o 1000 tokenech) | SentencePiece | 14,1 s | 0,1 s |
| 6 MB textu, 1 000 000 slov (272 000 různých) | BPE | – | 1,2 s |
| 6 MB textu | WordPiece | – | 0,4 s |
| 6 MB textu (60 000 řádků) | SentencePiece | – | 1,0 s |

Výsledky programu a webu se u všech měřených případů shodují do posledního kroku
(dvojice, symboly, četnosti i poměry).

## Sestavení ze zdrojů

Potřebujete [Go](https://go.dev/dl/) 1.24 nebo novější.

```
go test ./...        # testy
go build -o llm .    # sestavení pro tento počítač
./build.sh           # programy pro Windows, macOS a Linux do složky bin/
```
