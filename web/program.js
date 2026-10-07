// Doplňky, které program přidává k webové stránce při zobrazení v okně: rychlý výpočet slovníku
// v programu, vlastní text (karta Trénink), ukončení programu a hlídání, zda je okno otevřené.
(function(){
  'use strict';
  var TOKEN = window.__LLM_TOKEN__;

  function call(method, path, body, cb){
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
    if(typeof url === 'string' && url.indexOf('slovnik-tokenu.html') === 0){
      url += (url.indexOf('?') < 0 ? '?' : '&') + 't=' + TOKEN;
    }
    return origOpen.call(window, url, name, features);
  };

  // program běží, dokud je stránka otevřená
  setInterval(function(){ call('GET', '/api/ping', undefined, function(){}); }, 5000);
  window.addEventListener('pagehide', function(){
    if(navigator.sendBeacon) navigator.sendBeacon('/api/closing?t=' + TOKEN, '');
  });
  call('GET', '/api/ping', undefined, function(){});

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
  wrap.appendChild(quitRow);

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
    fileIn.accept = '.txt,.md,.csv,.tsv,.html,.json,text/*';
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
    call('GET', '/api/initial', undefined, function(st, d){
      if(d && d.text){ area.value = ''; use(d.text, d.name); }
    });
  }
})();
