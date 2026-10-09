// Doplňky, které program přidává k webové stránce při zobrazení v okně: rychlý výpočet slovníku
// v programu, vlastní text (karta Trénink), ukončení programu a hlídání, zda je okno otevřené.
(function(){
  'use strict';
  var TOKEN = window.__LLM_TOKEN__;
  // Webová stránka bez programu: výpočty dělá WebAssembly ve workeru (web/web-transport.js).
  var W = window.__LLM_WEB__ || null;
  var LOST = W ? 'Výpočet se přerušil. Obnovte stránku.' : 'Spojení s programem se přerušilo. Spusťte program znovu.';

  function call(method, path, body, cb){
    if(W){ W.call(method, path, body, cb); return; }
    var x = new XMLHttpRequest();
    x.open(method, path, true);
    x.setRequestHeader('X-Token', TOKEN);
    x.onload = function(){ var d = null; try{ d = JSON.parse(x.responseText); }catch(e){} cb(x.status, d); };
    x.onerror = function(){ cb(0, null); };
    x.send(body === undefined ? null : JSON.stringify(body));
  }

  // rychlý výpočet slovníku (stránka ho zavolá místo výpočtu v prohlížeči)
  window.llmTrain = function(req, cb){
    call('POST', '/api/train', req, function(st, d){
      if(!d){ cb(null); return; }
      if(d.error){ cb({error: d.error, chars: d.chars}); return; }
      cb({
        initial: d.initial,
        tokens: d.tokens,
        exhausted: !!d.exhausted,
        words: d.words,
        merges: d.merges.map(function(m){
          return {a: m.a, b: m.b, sym: m.new, count: m.count, ca: m.countA, cb: m.countB, dup: !!m.dup};
        })
      });
    });
  };

  // stránka se slovníkem tokenů se otevírá s přístupovým klíčem
  var origOpen = window.open;
  window.open = function(url, name, features){
    if(!W && typeof url === 'string' && url.indexOf('slovnik-tokenu.html') === 0){
      url += (url.indexOf('?') < 0 ? '?' : '&') + 't=' + TOKEN;
    }
    return origOpen.call(window, url, name, features);
  };

  // program běží, dokud je stránka otevřená
  if(!W){
    setInterval(function(){ call('GET', '/api/ping', undefined, function(){}); }, 5000);
    window.addEventListener('pagehide', function(){
      if(navigator.sendBeacon) navigator.sendBeacon('/api/closing?t=' + TOKEN, '');
    });
    call('GET', '/api/ping', undefined, function(){});
  }

  // tlačítko pro ukončení programu
  var wrap = document.querySelector('.wrap');
  var quitRow = document.createElement('div');
  quitRow.style.cssText = 'text-align:center;margin-top:18px;';
  var quit = document.createElement('button');
  quit.className = 'btn';
  quit.type = 'button';
  quit.textContent = 'Ukončit program';
  quit.addEventListener('click', function(){
    call('POST', '/api/quit', {}, function(){
      document.body.innerHTML = '<div style="text-align:center;padding:80px 20px;font-family:Work Sans,sans-serif;color:#5d6b76">Program byl ukončen. Toto okno můžete zavřít.</div>';
    });
  });
  quitRow.appendChild(quit);
  if(!W) wrap.appendChild(quitRow);

  // Text (korpus) na kartě Tokenizace: načíst ze souboru nebo vložit; objeví se dole na stránce
  // (u každého algoritmu) a hledá se v něm i tvoří slovník. Původní text o Albertu Einsteinovi
  // lze vrátit.
  var view = document.getElementById('view-tokenizace');
  if(view && window.llmCorpus){
    var panel = document.createElement('div');
    panel.className = 'panel';
    panel.innerHTML =
      '<div class="panel-title">Text (korpus)</div>' +
      '<div id="corpus-bar" style="display:flex;flex-wrap:wrap;gap:10px;align-items:center;"></div>' +
      '<div id="corpus-paste" hidden style="margin-top:12px;">' +
        '<textarea id="corpus-text" class="big-text" spellcheck="false" placeholder="Sem vložte libovolně dlouhý text…"></textarea>' +
        '<div style="margin-top:10px;"></div>' +
      '</div>' +
      '<p class="hint" id="corpus-status" style="margin:10px 0 0;"></p>';
    view.insertBefore(panel, view.firstChild);

    var bar = panel.querySelector('#corpus-bar');
    var pasteBox = panel.querySelector('#corpus-paste');
    var area = panel.querySelector('#corpus-text');
    var status = panel.querySelector('#corpus-status');
    function button(parent, text, cls){
      var b = document.createElement('button');
      b.type = 'button';
      b.className = 'btn' + (cls ? ' ' + cls : '');
      b.textContent = text;
      parent.appendChild(b);
      return b;
    }
    var fileBtn = button(bar, 'Načíst ze souboru…', '');
    var pasteBtn = button(bar, 'Vložit text…', '');
    var resetBtn = button(bar, 'Vrátit text o Albertu Einsteinovi', '');
    var useBtn = button(pasteBox.lastChild, 'Použít tento text', 'primary');
    var fileIn = document.createElement('input');
    fileIn.type = 'file';
    fileIn.accept = '.txt,.md,.csv,.tsv,.html,.json,.pdf,text/*,application/pdf';
    fileIn.hidden = true;
    bar.appendChild(fileIn);

    function say(text){ status.textContent = text; }
    say('Teď se pracuje s textem o Albertu Einsteinovi (předpřipravený, dole na stránce). Můžete načíst nebo vložit vlastní text.');

    function use(text, name){
      if(!text.trim()){ say('Vložte text nebo ho načtěte ze souboru.'); return; }
      window.llmCorpus.set(text);
      say('Teď se pracuje s textem: ' + name + ' (' + text.length.toLocaleString('cs-CZ') + ' znaků). Zobrazen je dole na stránce, zvýrazňuje se v něm hledané a z něj se tvoří slovník.');
    }
    fileBtn.addEventListener('click', function(){ fileIn.click(); });
    fileIn.addEventListener('change', function(){
      var f = fileIn.files[0];
      if(!f) return;
      if(/\.pdf$/i.test(f.name) || f.type === 'application/pdf'){
        say('Čte se text z PDF…');
        var done = function(d){
          if(d && d.text){ area.value = d.text; pasteBox.hidden = true; use(d.text, f.name); }
          else say((d && d.error) || 'PDF se nepodařilo přečíst.');
        };
        if(W){ W.pdf(f, done); fileIn.value = ''; return; }
        var x = new XMLHttpRequest();
        x.open('POST', '/api/pdf', true);
        x.setRequestHeader('X-Token', TOKEN);
        x.onload = function(){
          var d = null; try{ d = JSON.parse(x.responseText); }catch(e){}
          done(d);
        };
        x.onerror = function(){ say('PDF se nepodařilo přečíst.'); };
        x.send(f);
        fileIn.value = '';
        return;
      }
      var r = new FileReader();
      r.onload = function(){ area.value = r.result; pasteBox.hidden = true; use(r.result, f.name); };
      r.onerror = function(){ say('Soubor se nepodařilo přečíst.'); };
      r.readAsText(f, 'UTF-8');
      fileIn.value = '';
    });
    pasteBtn.addEventListener('click', function(){
      pasteBox.hidden = !pasteBox.hidden;
      if(!pasteBox.hidden) area.focus();
    });
    useBtn.addEventListener('click', function(){ use(area.value, 'vložený text'); });
    resetBtn.addEventListener('click', function(){
      window.llmCorpus.reset();
      area.value = '';
      say('Teď se pracuje s textem o Albertu Einsteinovi (předpřipravený).');
    });

    // soubor přetažený na program
    if(!W) call('GET', '/api/initial', undefined, function(st, d){
      if(d && d.text){ area.value = ''; use(d.text, d.name); }
    });
  }
  // Karta Trénink: matice společného výskytu tokenů. Používá slovník, který je právě vytvořený na
  // kartě Tokenizace (BPE, WordPiece nebo SentencePiece), a text, ze kterého vznikl.
  var trenink = document.getElementById('view-trenink');
  if(trenink && window.llmCorpus){
    var ALGOS = [['bpe', 'BPE'], ['wordpiece', 'WordPiece'], ['sentencepiece', 'SentencePiece']];
    var vocabs = {};        // algoritmus -> {n: počet tokenů} pro slovník, který je teď vytvořený
    var shownAlgo = null;   // algoritmus matice, která je zobrazená níže

    function h(tag, cls, text){
      var e = document.createElement(tag);
      if(cls) e.className = cls;
      if(text !== undefined) e.textContent = text;
      return e;
    }
    function fmt(n){ return Number(n).toLocaleString('cs-CZ'); }

    var mp = h('div', 'panel');
    mp.appendChild(h('div', 'panel-title', 'Matice společného výskytu'));
    mp.appendChild(h('p', 'hint',
      'Program projde text token po tokenu (tokeny podle vybraného slovníku). Pro každou pozici t vezme token w a ke každému tokenu c ' +
      'na pozicích t−Δ, …, t−1, t+1, …, t+Δ zvýší hodnotu M[w, c] o 1; samotná pozice t se nepočítá. ' +
      'Řádek matice je číslo tokenu w, sloupec číslo tokenu c (čísla jako ve slovníku, od 1). ' +
      'M[w, c] je tedy počet, kolikrát se token c v celém textu vyskytl do vzdálenosti Δ od tokenu w.'));

    var algoBox = h('div', 'field');
    algoBox.appendChild(h('label', '', 'Slovník'));
    var radios = {}, statuses = {};
    ALGOS.forEach(function(a){
      var row = h('div');
      row.style.cssText = 'margin:4px 0;';
      var lab = h('label');
      lab.style.cssText = 'display:inline-flex;gap:8px;align-items:center;font-weight:500;color:inherit;margin:0;cursor:pointer;';
      var r = document.createElement('input');
      r.type = 'radio';
      r.name = 'mx-algo';
      r.value = a[0];
      lab.appendChild(r);
      lab.appendChild(document.createTextNode(a[1]));
      var st = h('span', 'hint');
      st.style.cssText = 'margin-left:10px;';
      row.appendChild(lab);
      row.appendChild(st);
      algoBox.appendChild(row);
      radios[a[0]] = r;
      statuses[a[0]] = st;
    });
    mp.appendChild(algoBox);

    var deltaField = h('div', 'field');
    deltaField.style.cssText = 'max-width:220px;margin-top:12px;';
    var deltaLabel = h('label', '', 'Šířka okna Δ');
    deltaLabel.setAttribute('for', 'mx-delta');
    var deltaWrap = h('div', 'input-wrap');
    var deltaIn = document.createElement('input');
    deltaIn.id = 'mx-delta';
    deltaIn.type = 'number';
    deltaIn.min = '1';
    deltaIn.max = '1000';
    deltaIn.step = '1';
    deltaIn.value = '2';
    deltaIn.setAttribute('inputmode', 'numeric');
    deltaWrap.appendChild(deltaIn);
    deltaField.appendChild(deltaLabel);
    deltaField.appendChild(deltaWrap);
    mp.appendChild(deltaField);

    var mxBtnRow = h('div');
    mxBtnRow.style.cssText = 'margin-top:14px;';
    var mxBtn = h('button', 'btn primary', 'Vytvořit matici společného výskytu');
    mxBtn.type = 'button';
    mxBtnRow.appendChild(mxBtn);
    mp.appendChild(mxBtnRow);
    var mxStatus = h('p', 'hint');
    mxStatus.style.cssText = 'margin:10px 0 0;';
    mp.appendChild(mxStatus);
    var mxOut = h('div');
    mp.appendChild(mxOut);
    trenink.appendChild(mp);

    var NEEDS_VOCAB = 'Nejdřív vytvořte slovník na kartě Tokenizace.';
    function selectedAlgo(){
      for(var k in radios){ if(radios[k].checked && !radios[k].disabled) return k; }
      return null;
    }
    // zaškrtnout lze jen slovník, který je teď vytvořený
    function refresh(){
      var firstOk = null;
      ALGOS.forEach(function(a){
        var ok = !!vocabs[a[0]];
        radios[a[0]].disabled = !ok;
        if(!ok) radios[a[0]].checked = false;
        statuses[a[0]].textContent = ok
          ? 'vytvořen, ' + fmt(vocabs[a[0]].n) + ' tokenů'
          : 'zatím není vytvořen (vytvořte ho na kartě Tokenizace)';
        if(ok && !firstOk) firstOk = a[0];
      });
      if(!selectedAlgo() && firstOk) radios[firstOk].checked = true;
      mxBtn.disabled = !firstOk;
      if(!firstOk) mxStatus.textContent = NEEDS_VOCAB;
      else if(mxStatus.textContent === NEEDS_VOCAB) mxStatus.textContent = '';
    }
    function clearMatrix(){
      mxOut.innerHTML = '';
      shownAlgo = null;
    }

    // stránka tokenizace volá window.llmTrain při každém vytvoření slovníku: sledujeme, který slovník je hotový
    var rawTrain = window.llmTrain;
    window.llmTrain = function(req, cb){
      delete vocabs[req.algo];
      if(shownAlgo === req.algo) clearMatrix();
      refresh();
      rawTrain(req, function(res){
        if(res && !res.error && res.tokens) vocabs[req.algo] = {n: res.tokens.length};
        refresh();
        cb(res);
      });
    };
    // nový text znamená, že dosavadní slovníky a matice už k němu nepatří
    ['set', 'reset'].forEach(function(name){
      var orig = window.llmCorpus[name];
      window.llmCorpus[name] = function(){
        vocabs = {};
        clearMatrix();
        refresh();
        return orig.apply(this, arguments);
      };
    });

    function showMatrixView(){
      var kIn = mxOut.querySelector('#mx-k');
      var k = parseInt(kIn.value, 10);
      if(!(k >= 1)) k = 15;
      if(k > 100) k = 100;
      kIn.value = String(k);
      var mode = mxOut.querySelector('#mx-mode').value;
      var box = mxOut.querySelector('#mx-table');
      call('GET', '/api/matrix/view?k=' + k + '&mode=' + mode, undefined, function(st, d){
        box.innerHTML = '';
        if(!d || d.error){ box.appendChild(h('p', 'hint', (d && d.error) || LOST)); return; }
        var wrapT = h('div', 'vocab-scroll');
        var table = h('table', 'theory-table');
        var thead = h('thead');
        var hr = h('tr');
        var corner = h('th', 'mono', 'w \\ c');
        corner.style.textTransform = 'none';
        hr.appendChild(corner);
        d.tokens.forEach(function(t){
          var th = h('th', 'mono');
          th.style.textTransform = 'none';   // tokeny se zobrazují přesně (na velikosti písmen záleží)
          th.appendChild(document.createTextNode(t.token));
          th.appendChild(document.createElement('br'));
          var sm = h('small', '', String(t.n));
          sm.style.cssText = 'font-weight:400;opacity:.7;';
          th.appendChild(sm);
          hr.appendChild(th);
        });
        thead.appendChild(hr);
        table.appendChild(thead);
        var tbody = h('tbody');
        d.cells.forEach(function(row, i){
          var tr = h('tr');
          var th = h('th', 'mono');
          th.style.cssText = 'text-align:left;white-space:nowrap;';
          th.appendChild(document.createTextNode(d.tokens[i].token + ' '));
          var sm = h('small', '', String(d.tokens[i].n));
          sm.style.cssText = 'font-weight:400;opacity:.7;';
          th.appendChild(sm);
          tr.appendChild(th);
          row.forEach(function(x){
            var td = h('td', 'mono', String(x));
            if(x === 0) td.style.opacity = '.35';
            tr.appendChild(td);
          });
          tbody.appendChild(tr);
        });
        table.appendChild(tbody);
        wrapT.appendChild(table);
        box.appendChild(wrapT);
        box.appendChild(h('p', 'hint', 'Zobrazeno ' + d.tokens.length + ' z ' + fmt(d.size) + ' tokenů; celou matici si stáhněte jako soubor.'));
      });
    }

    function showMatrix(d, algoName){
      mxOut.innerHTML = '';
      var sum = h('p', 'vocab-sum');
      sum.style.cssText = 'margin-top:14px;';
      sum.appendChild(document.createTextNode('Matice '));
      sum.appendChild(h('b', '', fmt(d.size) + ' × ' + fmt(d.size)));
      sum.appendChild(document.createTextNode(' (slovník ' + algoName + ', Δ = ' + d.delta + '). Text má ' + fmt(d.textTokens) +
        ' tokenů, nenulových buněk je ' + fmt(d.nonzero) + ' z ' + fmt(d.size * d.size) + ', součet všech hodnot je ' + fmt(d.total) +
        '. Matice je souměrná (M[w, c] = M[c, w]). Spočítáno za ' + d.ms + ' ms.'));
      mxOut.appendChild(sum);

      var dl = h('button', 'btn primary', 'Stáhnout celou matici (.tsv)');
      dl.type = 'button';
      dl.addEventListener('click', function(){
        if(W){ W.download(function(err){ if(err) mxStatus.textContent = err; }); return; }
        var a = document.createElement('a');
        a.href = '/api/matrix/download?t=' + encodeURIComponent(TOKEN);
        a.setAttribute('download', '');
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
      });
      var dlRow = h('div');
      dlRow.style.cssText = 'margin:10px 0;';
      dlRow.appendChild(dl);
      dlRow.appendChild(h('span', 'hint', ' První řádek a první sloupec obsahují tokeny, pořadí odpovídá číslům tokenů ve slovníku.'));
      mxOut.appendChild(dlRow);

      var ctl = h('div');
      ctl.style.cssText = 'display:flex;flex-wrap:wrap;gap:10px;align-items:center;margin:10px 0;';
      ctl.appendChild(document.createTextNode('Zobrazit'));
      var kWrap = h('div', 'input-wrap');
      kWrap.style.cssText = 'width:84px;';
      var kIn = document.createElement('input');
      kIn.id = 'mx-k';
      kIn.type = 'number';
      kIn.min = '1';
      kIn.max = '100';
      kIn.value = String(Math.min(15, d.size));
      kWrap.appendChild(kIn);
      ctl.appendChild(kWrap);
      ctl.appendChild(document.createTextNode('tokenů:'));
      var sel = document.createElement('select');
      sel.id = 'mx-mode';
      [['freq', 'nejčastějších (v okolí jiných tokenů)'], ['index', 'prvních podle čísla ve slovníku']].forEach(function(o){
        var op = document.createElement('option');
        op.value = o[0];
        op.textContent = o[1];
        sel.appendChild(op);
      });
      ctl.appendChild(sel);
      var go = h('button', 'btn', 'Zobrazit');
      go.type = 'button';
      go.addEventListener('click', showMatrixView);
      ctl.appendChild(go);
      mxOut.appendChild(ctl);
      kIn.addEventListener('keydown', function(e){ if(e.key === 'Enter'){ e.preventDefault(); showMatrixView(); } });
      sel.addEventListener('change', showMatrixView);
      var box = h('div');
      box.id = 'mx-table';
      mxOut.appendChild(box);
      showMatrixView();
    }

    mxBtn.addEventListener('click', function(){
      var algo = selectedAlgo();
      if(!algo){ mxStatus.textContent = 'Vyberte slovník (vytvořte ho na kartě Tokenizace).'; return; }
      var raw = deltaIn.value.trim();
      var delta = Number(raw);
      if(!/^\d+$/.test(raw) || delta < 1 || delta > 1000){
        mxStatus.textContent = 'Δ musí být celé číslo od 1 do 1000.';
        return;
      }
      mxStatus.textContent = 'Počítám…';
      mxBtn.disabled = true;
      mxOut.innerHTML = '';
      call('POST', '/api/matrix', {algo: algo, delta: delta}, function(st, d){
        mxStatus.textContent = '';
        refresh();
        if(!d){ mxStatus.textContent = LOST; return; }
        if(d.error){ mxStatus.textContent = d.error; return; }
        shownAlgo = algo;
        showMatrix(d, ALGOS.filter(function(a){ return a[0] === algo; })[0][1]);
      });
    });
    refresh();
  }
})();
