#!/bin/sh
# Sestaví webovou stránku do složky docs/ (pro GitHub Pages i jakýkoli statický hosting).
# Výpočty dělá Go přeložené do WebAssembly přímo v prohlížeči – žádný server ani program.
# Potřebuje nainstalované Go. Zobrazení: ./build-web.sh && cd docs && python3 -m http.server
set -e
cd "$(dirname "$0")"
out=docs
rm -rf "$out"
mkdir -p "$out/spolecne"
GOOS=js GOARCH=wasm go build -trimpath -ldflags "-s -w" -o "$out/llm.wasm" .
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" "$out/wasm_exec.js" 2>/dev/null ||
	cp "$(go env GOROOT)/misc/wasm/wasm_exec.js" "$out/wasm_exec.js"
cp web/worker.js web/web-transport.js "$out/"
cp web/spolecne/styl.css "$out/spolecne/styl.css"
touch "$out/.nojekyll"
# hlavní stránka = albert-einstein.html + doplňky (stejné jako v okně programu) + přenos přes worker
python3 - <<'PY'
import re
src = open('web/albert-einstein.html', encoding='utf-8').read()
js = open('web/program.js', encoding='utf-8').read()
inject = '<script src="web-transport.js"></script>\n<script>\n' + js + '\n</script>\n</body>'
assert '</body>' in src
open('docs/index.html', 'w', encoding='utf-8').write(src.replace('</body>', inject, 1))
voc = open('web/slovnik-tokenu.html', encoding='utf-8').read()
assert 'href="albert-einstein.html"' in voc
open('docs/slovnik-tokenu.html', 'w', encoding='utf-8').write(voc.replace('href="albert-einstein.html"', 'href="index.html"', 1))
PY
echo "Hotovo: $out/ (otevřete přes http, např. cd $out && python3 -m http.server)"
