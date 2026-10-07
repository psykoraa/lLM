# Příklady

`einstein.txt` – text anglické Wikipedie o Albertu Einsteinovi (od úvodu po kapitolu *Awards and honors*),
jeden odstavec nebo nadpis na řádek. Stejný text je na webové stránce `albert-einstein.html`.

Zdroj: Wikipedia contributors, „Albert Einstein“, https://en.wikipedia.org/wiki/Albert_Einstein,
licence [CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/). Staženo 7. 10. 2026.

Vyzkoušení:

```
llm train -algo bpe -size 5000 -ignore-case -o bpe.tsv priklady/einstein.txt
```
