#!/bin/sh
# Zkopíruje webovou stránku (albert-einstein.html, slovnik-tokenu.html, styly) z repozitáře matematika
# do složky web/, ze které ji program zabudovává do okna v prohlížeči.
# Použití: ./sync-web.sh /cesta/k/repozitari/matematika
set -e
src="${1:?zadejte cestu k repozitáři matematika}"
cd "$(dirname "$0")"
mkdir -p web/spolecne
cp "$src/albert-einstein.html" web/albert-einstein.html
cp "$src/slovnik-tokenu.html" web/slovnik-tokenu.html
cp "$src/spolecne/styl.css" web/spolecne/styl.css
echo "Zkopírováno. Nezapomeňte přebudovat programy: ./build.sh"
