package opnborg

import _ "embed"

//go:embed resources/borg.png
var _favicon []byte

// see api.go
var _head, _forceRedirect string

const (
	_lf = "\n"

	_htmlStart = "<!doctype html><html>" + _lf
	_htmlEnd   = "</html>"

	_bodyStart = "<body>" + _lf
	_bodyEnd   = "</body>" + _lf

	_bodyHead   = "<header class=\"app-header\"><h1>" + _app + "</h1><div class=\"semver\">[ " + SemVer + " ]</div></header>" + _lf
	_bodyFooter = "<footer><div class=\"footer-links\"><a href=\"https://paepcke.de/opnborg\">" + _git + "</a><a href=\"https://infosec.exchange/@paepcke\">" + _social + "</a></div><div class=\"footer-sponsor\">SPONSORED-BY: <a href=\"https://pvz.digital\">pvz.digital</a> <a href=\"https://debitor.de\">debitor.de</a></div><div class=\"footer-tag\">RESISTANCE IS FUTILE. YOUR OPNSENSE WILL BE ASSIMILATED.</div></footer>" + _lf

	_forceInfo    = "<div class=\"force-info\"><h2>[ performing backup ]</h2><p>wait for redirect</p></div>"
	_forceButton  = "<a href=\"./force\" class=\"btn btn-force\">[ Backup NOW ]</a>"
	_configButton = "<a href=\"./config\" class=\"btn btn-force\" target=\"_blank\">[ Config Dashboard ]</a>"

	// _forceDashboard is the animated forced-backup progress screen. It
	// replaces the static "wait for redirect" page and streams the live log
	// lines the backup pass emits (polled from /progress) into a terminal
	// console, with a pulsing status, animated progress bar and live stats.
	// The %FG% / %BG% tokens are substituted at Setup() time from the
	// OPN_HTTPD_COLOR_FG / OPN_HTTPD_COLOR_BG theme colors so the dashboard
	// inherits the operator's configured palette.
	_forceDashboard = `<section class="force-dash" id="force-dash">
<div class="fd-orbit"><div class="fd-radar"></div><div class="fd-ring"><span class="fd-orb"></span><span class="fd-orb"></span><span class="fd-orb"></span><span class="fd-orb"></span><span class="fd-orb"></span></div><div class="fd-core"></div></div>
<div class="force-dash-inner">
<div class="fd-status"><span class="fd-tag" id="fd-status-tag">[ performing backup ]</span><span class="fd-elapsed" id="fd-elapsed">00:00</span></div>
<div class="fd-progress"><div class="fd-progress-bar" id="fd-progress-bar"></div><div class="fd-progress-pct" id="fd-progress-pct">0%</div></div>
<div class="fd-stats">
<div class="fd-stat"><span class="fd-stat-n" id="fd-lines">0</span><span class="fd-stat-l">log lines</span></div>
<div class="fd-stat"><span class="fd-stat-n" id="fd-servers">0</span><span class="fd-stat-l">servers</span></div>
<div class="fd-stat"><span class="fd-stat-n" id="fd-changes">0</span><span class="fd-stat-l">changes</span></div>
<div class="fd-stat"><span class="fd-stat-n" id="fd-errors">0</span><span class="fd-stat-l">errors</span></div>
</div>
<div class="fd-console-wrap">
<div class="fd-console-head">live backup stream</div>
<div class="fd-console" id="fd-console"><div class="fd-line fd-muted">waiting for backup pass to start...</div></div>
</div>
<div class="fd-hint" id="fd-hint">redirecting back to the hive view when the pass completes</div>
</div>
</section>
<section class="fd-done-box" id="fd-done-overlay">
<div class="fd-done-confetti"><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span><span></span></div>
<div class="fd-done-ring"></div>
<div class="fd-done-title" id="fd-done-title">backup complete</div>
<div class="fd-done-summary" id="fd-done-summary"></div>
<div class="fd-done-hint" id="fd-done-hint">returning to the hive view in <span id="fd-countdown">6</span>s &#8212; click anywhere to extend</div>
</section>
<style>
.force-dash{position:relative;max-width:920px;margin:1.5rem auto;padding:1.25rem;background:var(--card);border:1px solid var(--border);border-radius:12px;box-shadow:0 0 0 1px rgba(74,158,255,.05),0 8px 32px rgba(0,0,0,.35);overflow:hidden}
.force-dash::before{content:"";position:absolute;inset:0;background:radial-gradient(circle at 50% -10%,rgba(74,158,255,.16),transparent 55%);pointer-events:none;animation:fd-glow 3.6s ease-in-out infinite}
.force-dash-inner{position:relative;display:flex;flex-direction:column;gap:1rem;z-index:1}
.fd-orbit{position:relative;width:96px;height:96px;margin:0 auto .25rem;display:flex;align-items:center;justify-content:center}
.fd-core{width:16px;height:16px;border-radius:50%;background:var(--accent);box-shadow:0 0 12px var(--accent),0 0 26px rgba(74,158,255,.55);animation:fd-corepulse 1.5s ease-in-out infinite}
.fd-ring{position:absolute;inset:0;animation:fd-rotate 3.4s linear infinite}
.fd-orb{position:absolute;top:50%;left:50%;width:9px;height:9px;margin:-4.5px 0 0 -4.5px;border-radius:50%;background:var(--accent);box-shadow:0 0 8px var(--accent);animation:fd-orbpulse 1.4s ease-in-out infinite}
.fd-orb:nth-child(1){transform:rotate(0deg) translateX(38px)}
.fd-orb:nth-child(2){transform:rotate(72deg) translateX(38px);animation-delay:.15s}
.fd-orb:nth-child(3){transform:rotate(144deg) translateX(38px);animation-delay:.3s}
.fd-orb:nth-child(4){transform:rotate(216deg) translateX(38px);animation-delay:.45s}
.fd-orb:nth-child(5){transform:rotate(288deg) translateX(38px);animation-delay:.6s}
.fd-radar{position:absolute;inset:-6px;border-radius:50%;background:conic-gradient(from 0deg,transparent 0deg,rgba(74,158,255,.28) 38deg,transparent 78deg);animation:fd-rotate 2.6s linear infinite;opacity:.65;pointer-events:none}
.fd-done .fd-core{background:var(--ok);box-shadow:0 0 12px var(--ok),0 0 26px rgba(63,185,80,.55);animation:fd-pop .6s ease-out}
.fd-done .fd-orb{background:var(--ok);box-shadow:0 0 8px var(--ok);animation:none}
.fd-done .fd-ring{animation:fd-rotate 7s linear infinite}
.fd-done .fd-radar{opacity:.3;animation-duration:6s}
.fd-status{display:flex;align-items:center;justify-content:space-between;gap:1rem;flex-wrap:wrap}
.fd-tag{font-size:1.1rem;font-weight:600;color:var(--accent);letter-spacing:.08em;text-transform:uppercase;animation:fd-pulse 1.4s ease-in-out infinite}
.fd-done .fd-tag{color:var(--ok);animation:none}
.fd-elapsed{color:var(--muted);font-size:1.4rem;font-weight:700;font-variant-numeric:tabular-nums;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}
.fd-progress{position:relative;height:10px;background:var(--bg);border:1px solid var(--border);border-radius:99px;overflow:hidden}
.fd-progress-bar{position:absolute;inset:0 100% 0 0;background:linear-gradient(90deg,transparent,var(--accent),var(--accent),transparent);background-size:200% 100%;animation:fd-shimmer 1.6s linear infinite;border-radius:99px;transition:inset .25s ease-out}
.fd-done .fd-progress-bar{inset:0;background:var(--ok);animation:fd-shimmer 2.4s linear infinite}
.fd-progress-pct{position:absolute;right:.5rem;top:50%;transform:translateY(-50%);font-size:.6rem;color:var(--muted);font-variant-numeric:tabular-nums;pointer-events:none}
.fd-stats{display:grid;grid-template-columns:repeat(4,1fr);gap:.6rem}
.fd-stat{display:flex;flex-direction:column;align-items:center;padding:.5rem;background:var(--card-2);border:1px solid var(--border);border-radius:8px;transition:border-color .25s,box-shadow .25s}
.fd-stat-n{font-size:1.5rem;font-weight:700;color:var(--fg);font-variant-numeric:tabular-nums;line-height:1}
.fd-stat-l{color:var(--muted);font-size:.65rem;text-transform:uppercase;letter-spacing:.08em;margin-top:.2rem}
.fd-bump{animation:fd-bump .55s ease-out}
.fd-console-wrap{position:relative;border:1px solid var(--border);border-radius:8px;overflow:hidden;background:#0b0f14}
.fd-console-head{padding:.35rem .7rem;background:#141a22;color:var(--muted);font-size:.7rem;text-transform:uppercase;letter-spacing:.1em;border-bottom:1px solid var(--border);display:flex;align-items:center;gap:.4rem}
.fd-console-head::before{content:"";width:8px;height:8px;border-radius:50%;background:var(--accent);box-shadow:0 0 6px var(--accent);animation:fd-blink 1.2s ease-in-out infinite}
.fd-done .fd-console-head::before{background:var(--ok);box-shadow:0 0 6px var(--ok);animation:none}
.fd-console{height:300px;overflow-y:auto;padding:.5rem .7rem;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:.78rem;line-height:1.45;position:relative}
.fd-console::after{content:"";position:absolute;left:0;right:0;height:14px;background:linear-gradient(transparent,rgba(74,158,255,.07),transparent);animation:fd-scan 3.4s linear infinite;pointer-events:none}
.fd-done .fd-console::after{background:linear-gradient(transparent,rgba(63,185,80,.07),transparent)}
.fd-line{white-space:pre-wrap;word-break:break-all;padding:.1rem 0;border-bottom:1px solid rgba(255,255,255,.02);animation:fd-in .25s ease-out}
.fd-muted{color:var(--muted);font-style:italic}
.fd-ok{color:var(--ok)}
.fd-err{color:var(--err)}
.fd-warn{color:var(--warn)}
.fd-info{color:var(--accent)}
.fd-line .fd-ts{color:var(--muted);margin-right:.4rem;opacity:.7}
.fd-line .fd-br{color:var(--muted);margin-right:.3rem}
.fd-hint{color:var(--muted);font-size:.78rem;text-align:center}
.fd-done .fd-hint{color:var(--ok)}
.fd-done-box{position:relative;max-width:920px;margin:1.5rem auto;padding:1.5rem 1.25rem;background:var(--card);border:1px solid var(--ok);border-radius:12px;box-shadow:0 0 0 1px rgba(63,185,80,.08),0 8px 32px rgba(0,0,0,.35);display:flex;flex-direction:column;align-items:center;gap:.6rem;text-align:center;opacity:0;visibility:hidden;transform:scale(1.02);transition:opacity .35s ease-out,transform .35s ease-out,visibility .35s;overflow:hidden}
.fd-done-box.fd-show{opacity:1;visibility:visible;transform:scale(1)}
.fd-done-ring{width:54px;height:54px;border-radius:50%;border:3px solid var(--ok);box-shadow:0 0 18px rgba(63,185,80,.55),inset 0 0 12px rgba(63,185,80,.4);animation:fd-pop .6s ease-out,fd-ringrot 4s linear infinite}
.fd-done-title{font-size:1.4rem;font-weight:700;color:var(--ok);letter-spacing:.06em;text-transform:uppercase;animation:fd-pop .6s ease-out}
.fd-done-summary{color:var(--fg);font-size:.85rem;font-variant-numeric:tabular-nums;line-height:1.6}
.fd-done-summary b{color:var(--accent);font-weight:700}
.fd-done-hint{color:var(--muted);font-size:.8rem}
.fd-done-hint #fd-countdown{color:var(--ok);font-weight:700;font-size:1rem}
.fd-extend-flash{animation:fd-extend .4s ease-out}
.fd-done-confetti{position:absolute;inset:0;pointer-events:none;overflow:hidden}
.fd-done-confetti span{position:absolute;top:-12px;width:7px;height:12px;border-radius:2px;opacity:0;animation:fd-confetti 2.2s ease-in forwards}
.fd-done-confetti span:nth-child(1){left:8%;background:var(--accent);animation-delay:.05s}
.fd-done-confetti span:nth-child(2){left:18%;background:var(--ok);animation-delay:.15s}
.fd-done-confetti span:nth-child(3){left:28%;background:var(--warn);animation-delay:.25s}
.fd-done-confetti span:nth-child(4){left:38%;background:var(--accent);animation-delay:.1s}
.fd-done-confetti span:nth-child(5){left:48%;background:var(--ok);animation-delay:.3s}
.fd-done-confetti span:nth-child(6){left:58%;background:var(--accent);animation-delay:.2s}
.fd-done-confetti span:nth-child(7){left:68%;background:var(--warn);animation-delay:.05s}
.fd-done-confetti span:nth-child(8){left:78%;background:var(--ok);animation-delay:.25s}
.fd-done-confetti span:nth-child(9){left:88%;background:var(--accent);animation-delay:.15s}
.fd-done-confetti span:nth-child(10){left:24%;background:var(--ok);animation-delay:.35s}
.fd-done-confetti span:nth-child(11){left:54%;background:var(--warn);animation-delay:.4s}
.fd-done-confetti span:nth-child(12){left:74%;background:var(--accent);animation-delay:.3s}
@keyframes fd-pulse{0%,100%{opacity:1}50%{opacity:.55}}
@keyframes fd-glow{0%,100%{opacity:.55}50%{opacity:1}}
@keyframes fd-bounce{0%,100%{transform:translateY(0);opacity:.5}50%{transform:translateY(-8px);opacity:1}}
@keyframes fd-shimmer{0%{background-position:200% 0}100%{background-position:-200% 0}}
@keyframes fd-blink{0%,100%{opacity:1}50%{opacity:.3}}
@keyframes fd-in{from{opacity:0;transform:translateX(-6px)}to{opacity:1;transform:translateX(0)}}
@keyframes fd-rotate{from{transform:rotate(0deg)}to{transform:rotate(360deg)}}
@keyframes fd-corepulse{0%,100%{transform:scale(1);opacity:1}50%{transform:scale(1.45);opacity:.7}}
@keyframes fd-orbpulse{0%,100%{opacity:1}50%{opacity:.4}}
@keyframes fd-scan{0%{top:0}100%{top:100%}}
@keyframes fd-bump{0%{transform:scale(1)}35%{transform:scale(1.45);color:var(--accent);text-shadow:0 0 12px rgba(74,158,255,.6)}100%{transform:scale(1)}}
@keyframes fd-pop{0%{transform:scale(.4);opacity:0}60%{transform:scale(1.2);opacity:1}100%{transform:scale(1)}}
@keyframes fd-ringrot{from{transform:rotate(0deg)}to{transform:rotate(360deg)}}
@keyframes fd-extend{0%{transform:scale(1)}40%{transform:scale(1.12);color:var(--accent)}100%{transform:scale(1)}}
@keyframes fd-confetti{0%{opacity:0;transform:translateY(0) rotate(0deg)}10%{opacity:1}100%{opacity:0;transform:translateY(320px) rotate(540deg)}}
@media(max-width:640px){.fd-stats{grid-template-columns:repeat(2,1fr)}.fd-console{height:240px}}
</style>
<script>
(function(){
  const FORCE=%FORCE%;
  const POLL_MS=800;
  const MAX_WAIT_MS=180000;
  const HOLD_MS=6000;
  const CLICK_EXTEND_MS=4000;
  const el={
    root:document.getElementById('force-dash'),
    status:document.getElementById('fd-status-tag'),
    elapsed:document.getElementById('fd-elapsed'),
    bar:document.getElementById('fd-progress-bar'),
    pct:document.getElementById('fd-progress-pct'),
    console:document.getElementById('fd-console'),
    lines:document.getElementById('fd-lines'),
    servers:document.getElementById('fd-servers'),
    changes:document.getElementById('fd-changes'),
    errors:document.getElementById('fd-errors'),
    hint:document.getElementById('fd-hint'),
    overlay:document.getElementById('fd-done-overlay'),
    doneTitle:document.getElementById('fd-done-title'),
    doneSummary:document.getElementById('fd-done-summary'),
    doneHint:document.getElementById('fd-done-hint'),
    countdown:document.getElementById('fd-countdown')
  };
  let since=0, lineCount=0, srvCount=0, chCount=0, erCount=0, seen=new Set(), sawBusy=false, done=false, redirected=false;
  let holdDeadline=0, countdownTimer=null, fakePct=0;
  const esc=(s)=>s.replace(/[&<>]/g,(c)=>({'&':'&','<':'<','>':'>'}[c]));
  function classify(msg){
    if(/ERROR|FAIL|UNABLE/.test(msg))return'fd-err';
    if(/WARN/.test(msg))return'fd-warn';
    if(/OK|SUCCESS|FINISH|SUCCESSFUL|UPTODATE/.test(msg))return'fd-ok';
    if(/BACKUP|START|FETCH|GIT|SYNC|UNIFI|SERVICE/.test(msg))return'fd-info';
    return'';
  }
  function fmtMs(ms){
    const s=Math.max(0,Math.floor(ms/1000));
    return String(Math.floor(s/60)).padStart(2,'0')+':'+String(s%60).padStart(2,'0');
  }
  function bump(node,val){
    if(String(node.textContent)===String(val))return;
    node.textContent=val;
    node.classList.remove('fd-bump');
    void node.offsetWidth;
    node.classList.add('fd-bump');
  }
  function addLine(l){
    if(seen.has(l.seq))return;
    seen.add(l.seq);
    lineCount++;
    const d=new Date(l.ts);
    const ts=String(d.getHours()).padStart(2,'0')+':'+String(d.getMinutes()).padStart(2,'0')+':'+String(d.getSeconds()).padStart(2,'0');
    const div=document.createElement('div');
    div.className='fd-line '+classify(l.msg);
    div.innerHTML='<span class="fd-ts">'+ts+'</span><span class="fd-br">|</span>'+esc(l.msg);
    el.console.appendChild(div);
    el.console.scrollTop=el.console.scrollHeight;
  }
  function setStats(lines){
    for(const m of lines){
      if(/\[BACKUP\]\[START\]\[SERVER\]|\[BACKUP\]\[SERVER\]\[NO-CHANGE\]|\[BACKUP\]\[OK\]|\[BACKUP\]\[ERROR\]/.test(m.msg)){
        const mm=/SERVER\] ([^ ]+)/.exec(m.msg);if(mm&&!seen.has(mm[1]+'$')){seen.add(mm[1]+'$');srvCount++;}
      }
      if(/OK|SUCCESS|STORE-CHECKIN/.test(m.msg))chCount++;
      if(/ERROR|FAIL/.test(m.msg))erCount++;
    }
    bump(el.lines,lineCount);
    bump(el.servers,srvCount);
    bump(el.changes,chCount);
    bump(el.errors,erCount);
  }
  function renderSummary(){
    el.doneSummary.innerHTML=
      '<b>'+lineCount+'</b> log lines &middot; <b>'+srvCount+'</b> servers &middot; <b>'+chCount+'</b> changes &middot; <b>'+erCount+'</b> errors<br>'+
      'elapsed <b>'+el.elapsed.textContent+'</b>';
  }
  function finish(){
    if(done)return;
    done=true;
    el.root.classList.add('fd-done');
    el.status.textContent='[ backup complete ]';
    holdDeadline=Date.now()+HOLD_MS;
    fakePct=100;
    el.pct.textContent='100%';
    el.bar.style.inset='0 0 0 0';
    renderSummary();
    el.overlay.classList.add('fd-show');
    countdownTimer=setInterval(updateCountdown,200);
    updateCountdown();
  }
  function updateCountdown(){
    const remain=Math.max(0,Math.ceil((holdDeadline-Date.now())/1000));
    el.countdown.textContent=remain;
    if(Date.now()>=holdDeadline){
      clearInterval(countdownTimer);
      redirected=true;
      window.location.href='../';
    }
  }
  window.addEventListener('click',function(){
    if(!done)return;
    holdDeadline=Math.max(holdDeadline,Date.now())+CLICK_EXTEND_MS;
    el.doneHint.classList.remove('fd-extend-flash');
    void el.doneHint.offsetWidth;
    el.doneHint.classList.add('fd-extend-flash');
  });
  setInterval(function(){
    if(done){return;}
    if(fakePct<92){fakePct+=(92-fakePct)*0.05+0.15;}
    el.pct.textContent=Math.round(fakePct)+'%';
    el.bar.style.inset='0 '+(100-Math.min(100,fakePct))+'% 0 0';
  },140);
  async function poll(){
    if(redirected)return;
    try{
      const r=await fetch('progress?since='+since,{cache:'no-store',headers:{'Cache-Control':'no-store'}});
      const d=await r.json();
      const fresh=[];
      for(const l of d.lines||[]){if(l.seq>since){since=l.seq;fresh.push(l);}addLine(l);}
      setStats(fresh);
      el.elapsed.textContent=fmtMs(d.elapsed_ms||0);
      if(d.busy)sawBusy=true;
      if(sawBusy&&!d.busy){finish();return;}
      if(FORCE>0&&d.pass>=FORCE&&!d.busy){finish();return;}
    }catch(e){/* transient */}
    setTimeout(poll,POLL_MS);
  }
  setTimeout(()=>{if(!done){el.status.textContent='[ backup timed out ]';el.hint.textContent='timeout: redirecting back to the hive view';finish();}},MAX_WAIT_MS);
  poll();
})();
</script>`

	_git = `<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path stroke="none" d="M0 0h24v24H0z" fill="none" /><path d="M9 19c-4.3 1.4 -4.3 -2.5 -6 -3m12 5v-3.5c0 -1 .1 -1.4 -.5 -2c2.8 -.3 5.5 -1.4 5.5 -6a4.6 4.6 0 0 0 -1.3 -3.2a4.2 4.2 0 0 0 -.1 -3.2s-1.1 -.3 -3.5 1.3a12.3 12.3 0 0 0 -6.2 0c-2.4 -1.6 -3.5 -1.3 -3.5 -1.3a4.2 4.2 0 0 0 -.1 3.2a4.6 4.6 0 0 0 -1.3 3.2c0 4.6 2.7 5.7 5.5 6c-.6 .6 -.6 1.2 -.5 2v3.5" /></svg><span>Git</span>`

	_social = `<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path stroke="none" d="M0 0h24v24H0z" fill="none" /><path d="M18.648 15.254c-1.816 1.763 -6.648 1.626 -6.648 1.626a18.262 18.262 0 0 1 -3.288 -.256c1.127 1.985 4.12 2.81 8.982 2.475c-1.945 2.013 -13.598 5.257 -13.668 -7.636l-.026 -1.154c0 -3.036 .023 -4.115 1.352 -5.633c1.671 -1.91 6.648 -1.666 6.648 -1.666s4.977 -.243 6.648 1.667c1.329 1.518 1.352 2.597 1.352 5.633s-.456 4.074 -1.352 4.944z" /><path d="M12 11.204v-2.926c0 -1.258 -.895 -2.278 -2 -2.278s-2 1.02 -2 2.278v4.722m4 -4.722c0 -1.258 .895 -2.278 2 -2.278s2 1.02 2 2.278v4.722" /></svg><span>Mastodon</span>`

	_fail = `<svg xmlns="http://www.w3.org/2000/svg" width="1em" height="1em" viewBox="0 0 24 24"><rect width="10" height="10" x="1" y="1" fill="#a51d2d" rx="1"><animate id="svgSpinnersBlocksShuffle30" fill="freeze" attributeName="x" begin="0;svgSpinnersBlocksShuffle3b.end" dur="0.2s" values="1;13"/><animate id="svgSpinnersBlocksShuffle31" fill="freeze" attributeName="y" begin="svgSpinnersBlocksShuffle38.end" dur="0.2s" values="1;13"/><animate id="svgSpinnersBlocksShuffle32" fill="freeze" attributeName="x" begin="svgSpinnersBlocksShuffle39.end" dur="0.2s" values="13;1"/><animate id="svgSpinnersBlocksShuffle33" fill="freeze" attributeName="y" begin="svgSpinnersBlocksShuffle3a.end" dur="0.2s" values="13;1"/></rect><rect width="10" height="10" x="1" y="13" fill="#a51d2d" rx="1"><animate id="svgSpinnersBlocksShuffle34" fill="freeze" attributeName="y" begin="svgSpinnersBlocksShuffle30.end" dur="0.2s" values="13;1"/><animate id="svgSpinnersBlocksShuffle35" fill="freeze" attributeName="x" begin="svgSpinnersBlocksShuffle31.end" dur="0.2s" values="1;13"/><animate id="svgSpinnersBlocksShuffle36" fill="freeze" attributeName="y" begin="svgSpinnersBlocksShuffle32.end" dur="0.2s" values="1;13"/><animate id="svgSpinnersBlocksShuffle37" fill="freeze" attributeName="x" begin="svgSpinnersBlocksShuffle33.end" dur="0.2s" values="13;1"/></rect><rect width="10" height="10" x="13" y="13" fill="#a51d2d" rx="1"><animate id="svgSpinnersBlocksShuffle38" fill="freeze" attributeName="x" begin="svgSpinnersBlocksShuffle34.end" dur="0.2s" values="13;1"/><animate id="svgSpinnersBlocksShuffle39" fill="freeze" attributeName="y" begin="svgSpinnersBlocksShuffle35.end" dur="0.2s" values="13;1"/><animate id="svgSpinnersBlocksShuffle3a" fill="freeze" attributeName="x" begin="svgSpinnersBlocksShuffle36.end" dur="0.2s" values="1;13"/><animate id="svgSpinnersBlocksShuffle3b" fill="freeze" attributeName="y" begin="svgSpinnersBlocksShuffle37.end" dur="0.2s" values="1;13"/></rect><title>failed: unable to contact</title></svg>`

	_ok = `<?xml version="1.0" encoding="UTF-8" standalone="no"?><svg width="1em" height="1em" viewBox="0 0 252 252" version="1.1" xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" preserveAspectRatio="xMidYMid"><g fill="lime"><path d="M252.722963,5.10923976 C266.230721,18.6106793 228.784285,77.9582562 222.453709,84.2919919 C216.123132,90.6130917 200.043973,84.7974272 186.542533,71.2928287 C173.034776,57.7913892 167.215952,41.7090709 173.546529,35.3784942 C179.873947,29.0447585 239.218365,-8.39851769 252.722963,5.10923976"></path><path d="M63.3047797,19.3941039 C42.6830209,7.69327755 13.3393448,-5.32168052 4.00458729,4.01307703 C-5.4533701,13.4678754 8.0385925,43.4717763 19.8626187,64.1314428 C30.3851491,45.8378452 45.3523509,30.4378645 63.3047797,19.3941039"></path><path d="M232.123317,79.6356695 C234.021858,86.0768102 233.680689,91.3965163 230.600693,94.4701945 C223.407718,101.666329 203.976891,94.0058259 186.4604,77.3359391 C185.237878,76.2397763 184.024834,75.102547 182.833902,73.9084562 C176.500166,67.5715616 171.572172,60.8271597 168.41952,54.6166239 C162.284799,43.610771 160.74954,33.8906191 165.386908,29.2532506 C167.914085,26.7292332 171.957567,26.0405777 176.88872,26.9282483 C180.104551,24.8938714 183.901634,22.6288896 188.065157,20.3070464 C171.136234,11.4777241 151.888628,6.48970982 131.472202,6.48970982 C63.7533535,6.48970982 8.85360686,61.3799796 8.85360686,129.105146 C8.85360686,196.817677 63.7533535,251.714264 131.472202,251.714264 C199.191051,251.714264 254.087638,196.817677 254.087638,129.105146 C254.087638,107.235594 248.347789,86.7275581 238.321217,68.9488727 C236.154163,72.9039036 234.04713,76.5272426 232.123317,79.6356695"></path></g><title>hive member functional</title></svg>`

	_na = `<svg xmlns="http://www.w3.org/2000/svg" width="1em" height="1em" viewBox="0 0 24 24"><circle cx="4" cy="12" r="3" fill="currentColor"><animate id="svgSpinners3DotsBounce0" attributeName="cy" begin="0;svgSpinners3DotsBounce1.end+0.5s" calcMode="spline" dur="1.2s" keySplines=".33,.66,.66,1;.33,0,.66,.33" values="12;6;12"/></circle><circle cx="12" cy="12" r="3" fill="currentColor"><animate attributeName="cy" begin="svgSpinners3DotsBounce0.begin+0.2s" calcMode="spline" dur="1.2s" keySplines=".33,.66,.66,1;.33,0,.66,.33" values="12;6;12"/></circle><circle cx="20" cy="12" r="3" fill="currentColor"><animate id="svgSpinners3DotsBounce1" attributeName="cy" begin="svgSpinners3DotsBounce0.begin+0.4s" calcMode="spline" dur="1.2s" keySplines=".33,.66,.66,1;.33,0,.66,.33" values="12;6;12"/></circle><title>appliance status not yet available, please wait</title></svg>`

	_degraded = `<svg xmlns="http://www.w3.org/2000/svg" width="1em" height="1em" viewBox="0 0 24 24"><rect width="2.8" height="12" x="1" y="6" fill="#1c71d8"><animate attributeName="y" begin="svgSpinnersBarsScaleMiddle0.begin+0.4s" calcMode="spline" dur="0.6s" keySplines=".14,.73,.34,1;.65,.26,.82,.45" values="6;1;6"/><animate attributeName="height" begin="svgSpinnersBarsScaleMiddle0.begin+0.4s" calcMode="spline" dur="0.6s" keySplines=".14,.73,.34,1;.65,.26,.82,.45" values="12;22;12"/></rect><rect width="2.8" height="12" x="5.8" y="6" fill="#1c71d8"><animate attributeName="y" begin="svgSpinnersBarsScaleMiddle0.begin+0.2s" calcMode="spline" dur="0.6s" keySplines=".14,.73,.34,1;.65,.26,.82,.45" values="6;1;6"/><animate attributeName="height" begin="svgSpinnersBarsScaleMiddle0.begin+0.2s" calcMode="spline" dur="0.6s" keySplines=".14,.73,.34,1;.65,.26,.82,.45" values="12;22;12"/></rect><rect width="2.8" height="12" x="10.6" y="6" fill="#1c71d8"><animate id="svgSpinnersBarsScaleMiddle0" attributeName="y" begin="0;svgSpinnersBarsScaleMiddle1.end-0.1s" calcMode="spline" dur="0.6s" keySplines=".14,.73,.34,1;.65,.26,.82,.45" values="6;1;6"/><animate attributeName="height" begin="0;svgSpinnersBarsScaleMiddle1.end-0.1s" calcMode="spline" dur="0.6s" keySplines=".14,.73,.34,1;.65,.26,.82,.45" values="12;22;12"/></rect><rect width="2.8" height="12" x="15.4" y="6" fill="#1c71d8"><animate attributeName="y" begin="svgSpinnersBarsScaleMiddle0.begin+0.2s" calcMode="spline" dur="0.6s" keySplines=".14,.73,.34,1;.65,.26,.82,.45" values="6;1;6"/><animate attributeName="height" begin="svgSpinnersBarsScaleMiddle0.begin+0.2s" calcMode="spline" dur="0.6s" keySplines=".14,.73,.34,1;.65,.26,.82,.45" values="12;22;12"/></rect><rect width="2.8" height="12" x="20.2" y="6" fill="#1c71d8"><animate id="svgSpinnersBarsScaleMiddle1" attributeName="y" begin="svgSpinnersBarsScaleMiddle0.begin+0.4s" calcMode="spline" dur="0.6s" keySplines=".14,.73,.34,1;.65,.26,.82,.45" values="6;1;6"/><animate attributeName="height" begin="svgSpinnersBarsScaleMiddle0.begin+0.4s" calcMode="spline" dur="0.6s" keySplines=".14,.73,.34,1;.65,.26,.82,.45" values="12;22;12"/></rect><title>DEGRADED</title></svg>`

	_unifi = `<svg fill="#0559C9" role="img" width="1em" height="1em" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><title>Ubiquiti</title><path d="M23.1627 0h-1.4882v1.4882h1.4882zm-5.2072 10.4226V7.4409l.0007.001h2.9755v2.9762h2.9756v.9433c0 1.0906-.0927 2.3827-.306 3.3973-.1194.5672-.3004 1.1308-.5127 1.672-.2175.5537-.468 1.0841-.7408 1.5595a11.6795 11.6795 0 0 1-1.2456 1.7762l-.0253.0294-.0417.0488c-.1148.1347-.2283.2679-.3531.398a11.7612 11.7612 0 0 1-.4494.4492c-1.9046 1.8343-4.3861 2.98-6.9808 3.243-.3122.032-.939.0652-1.2519.0652-.3139-.001-.9397-.0331-1.252-.0651-2.5946-.263-5.0761-1.4097-6.9806-3.243a11.75 11.75 0 0 1-.4495-.4494c-.131-.1356-.249-.2748-.3683-.4154l-.0006-.0004-.0512-.0603a11.6576 11.6576 0 0 1-1.2456-1.7762c-.2727-.4763-.5233-1.0058-.7408-1.5595-.2123-.5414-.3933-1.1048-.5128-1.6718C.1854 13.743.0927 12.452.0927 11.3616V.1864h5.9518v10.2362s0 .7847.0099 1.0415l.0022.0599v.0004c.0127.332.0247.6575.0594.9812.098.919.3014 1.7913.7203 2.5288.1213.213.2443.42.3915.616.8953 1.1939 2.2577 2.0901 3.9573 2.3398.2022.0294.6108.0552.8149.0552.204 0 .6125-.0258.8149-.0552 1.6996-.2497 3.062-1.146 3.9573-2.3398.148-.196.2701-.403.3914-.616.419-.7375.6224-1.6095.7204-2.5288.0346-.3243.047-.6503.0594-.9831l.0022-.0584c.0099-.2568.0099-1.0415.0099-1.0415zm.7427-8.19h2.2326v2.2319h2.9764v2.9764h-2.9764V4.4654h-2.2326V2.2328Z"/></svg>`
)

// _css is the modern stylesheet injected into the HTML head at Setup() time.
// Theme colors are wired through CSS custom properties so that the
// OPN_HTTPD_COLOR_FG / OPN_HTTPD_COLOR_BG env vars still work.
const _css = `<style>
:root{--fg:%FG%;--bg:%BG%;--card:#1e2a3a;--card-2:#243447;--border:#2a3a5a;--accent:#4a9eff;--muted:#8a9aaa;--ok:#3fb950;--warn:#d29922;--err:#f85149}
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;background:var(--bg);color:var(--fg);max-width:1400px;margin:0 auto;padding:1rem;line-height:1.5}
.app-header{display:flex;align-items:center;justify-content:space-between;padding:1rem 0;border-bottom:2px solid var(--border)}
.app-header h1{font-size:1.6rem;letter-spacing:.05em;color:var(--accent)}
nav{display:flex;flex-wrap:wrap;gap:.5rem;padding:.75rem 0}
nav a{text-decoration:none}
nav a button{background:var(--card);color:var(--fg);border:1px solid var(--border);padding:.4rem .8rem;border-radius:6px;cursor:pointer;font-size:.85rem;transition:border-color .2s,background .2s}
nav a button:hover{border-color:var(--accent);background:var(--card-2)}
.semver{color:var(--muted);font-size:.8rem;margin:0;font-variant-numeric:tabular-nums}
.group{margin:1rem 0;padding:1rem;background:var(--card);border:1px solid var(--border);border-radius:8px}
.group-header{display:flex;align-items:baseline;gap:.5rem;flex-wrap:wrap;padding-bottom:.5rem;border-bottom:1px solid var(--border);margin-bottom:.5rem}
.group-header b{font-size:1.1rem;color:var(--accent)}
.group-desc{color:var(--muted);font-size:.85rem}
.group-img{max-width:100%;max-height:64px;object-fit:contain;display:block}
.member-row{display:flex;align-items:stretch;gap:.6rem;flex-wrap:wrap;padding:.5rem 0;border-bottom:1px solid rgba(255,255,255,.05)}
.member-row:last-child{border-bottom:none}
.member-status{display:flex;align-items:center;font-size:1.4rem;flex-shrink:0;min-width:1.6em;justify-content:center}
.member-main{display:flex;align-items:center;gap:.5rem;flex-wrap:wrap;flex:1 1 auto;min-width:200px}
.member-links{display:flex;gap:.3rem;flex-wrap:wrap;align-items:center}
.member-links a button{background:var(--bg);color:var(--fg);border:1px solid var(--border);padding:.25rem .5rem;border-radius:4px;cursor:pointer;font-size:.75rem;transition:border-color .2s,background .2s}
.member-links a button:hover{border-color:var(--accent);background:var(--card-2)}
.meta-box{display:flex;flex-direction:column;gap:.1rem;padding:.25rem .5rem;border:1px solid var(--border);border-radius:6px;background:var(--bg);min-width:120px;flex-shrink:0}
.meta-label{color:var(--muted);font-size:.65rem;text-transform:uppercase;letter-spacing:.08em}
.meta-value{color:var(--fg);font-size:.78rem;font-variant-numeric:tabular-nums;word-break:break-word}
.meta-last-seen{border-left:3px solid var(--accent)}
.meta-tag{border-left:3px solid var(--ok)}
.meta-sync{border-left:3px solid var(--ok)}
.meta-skip{border-left:3px solid var(--muted)}
.meta-file{border-left:3px solid var(--accent)}
.meta-err{border-left:3px solid var(--err)}
.backup-section{margin:1rem 0;padding:1rem;background:var(--card);border:1px solid var(--border);border-radius:8px}
.backup-section b{color:var(--accent)}
.btn-force{display:inline-block;margin:.5rem 0;padding:.5rem 1rem;background:var(--accent);color:#fff;border:none;border-radius:6px;cursor:pointer;text-decoration:none;font-weight:600}
.btn-force:hover{filter:brightness(1.1)}
.cfg-intro{color:var(--muted);font-size:.8rem;margin-bottom:.75rem}
.force-info{text-align:center;padding:2rem 0}
.force-info h2{color:var(--accent)}
footer{margin-top:2rem;padding:1rem 0;border-top:2px solid var(--border);display:flex;flex-direction:column;gap:.3rem}
.footer-links{display:flex;gap:1rem}
.footer-links a{color:var(--accent);text-decoration:none;display:flex;align-items:center;gap:.25rem;font-size:.85rem}
.footer-sponsor{color:var(--muted);font-size:.8rem}
.footer-sponsor a{color:var(--accent);text-decoration:none}
.footer-tag{color:var(--muted);font-size:.75rem;font-style:italic}
.dashboard{margin:1.5rem 0;padding:1rem;background:var(--card);border:1px solid var(--border);border-radius:8px}
.dashboard h2{font-size:1.1rem;color:var(--accent);margin-bottom:.75rem;letter-spacing:.05em}
.dashboard-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(240px,1fr));gap:.75rem}
.dash-panel{background:var(--card-2);border:1px solid var(--border);border-radius:6px;padding:.6rem .75rem}
.dash-title{color:var(--accent);font-size:.85rem;font-weight:600;text-transform:uppercase;letter-spacing:.08em;margin-bottom:.5rem;padding-bottom:.35rem;border-bottom:1px solid var(--border)}
.dash-row{display:flex;justify-content:space-between;gap:.5rem;padding:.18rem 0;font-size:.78rem}
.dash-label{color:var(--muted);flex-shrink:0}
.dash-value{color:var(--fg);text-align:right;word-break:break-word;font-variant-numeric:tabular-nums}
.dash-value:has(.target-chip){display:flex;flex-wrap:wrap;justify-content:flex-end;gap:.2rem .25rem;align-items:center}
.dash-row-below{flex-direction:column;align-items:flex-start;gap:.15rem}
.dash-value-below{text-align:left;display:flex;flex-wrap:wrap;justify-content:flex-start;gap:.2rem .25rem;align-items:center}
.dash-muted{color:var(--muted);font-style:italic}
.dash-ok{color:var(--ok)}
.dash-warn{color:var(--warn)}
.dash-err{color:var(--err)}
.target-chip{display:inline-block;background:var(--bg);border:1px solid var(--border);border-radius:4px;padding:.05rem .4rem;margin:.1rem .15rem .1rem 0;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:.72rem;white-space:nowrap}
.raw-env-val .target-chip{font-size:.72rem}
@media(max-width:640px){.member-row{flex-direction:column;align-items:flex-start}.member-main{flex-direction:column;align-items:flex-start}.meta-box{width:100%}.dashboard-grid{grid-template-columns:1fr}.raw-env-grid{grid-template-columns:1fr}}
.raw-env-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(320px,1fr));gap:.4rem;margin-top:.5rem}
.raw-env-row{display:flex;gap:.5rem;align-items:baseline;padding:.3rem .4rem;background:var(--card-2);border:1px solid var(--border);border-radius:4px}
.raw-env-name{color:var(--accent);font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:.75rem;flex:0 0 auto;word-break:break-all}
.raw-env-val{color:var(--fg);font-size:.78rem;word-break:break-word;text-align:right;flex:1 1 auto}
.raw-env-val:has(.target-chip){display:flex;flex-wrap:wrap;justify-content:flex-end;gap:.2rem .25rem;align-items:center}
.raw-env-val code{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:.75rem;background:var(--bg);padding:.05rem .25rem;border-radius:3px}
.raw-env-sub{color:var(--warn);font-size:.9rem;margin:1rem 0 .25rem 0}
.raw-env-unknown .raw-env-name{color:var(--err)}
.audit-tile{display:flex;flex-wrap:wrap;gap:.5rem;align-items:center}
.audit-tile .btn-force{margin:0}
.backup-tile{display:flex;flex-wrap:wrap;gap:.5rem;align-items:center}
.backup-tile .btn-force{margin:0;margin-left:auto}
.audit-page{margin:1.5rem 0;padding:1rem;background:var(--card);border:1px solid var(--border);border-radius:8px}
.audit-page h2{margin-bottom:.5rem}
nav .audit-active button{border-color:var(--accent);background:var(--card-2);color:var(--accent)}
.audit-list{display:flex;flex-direction:column;gap:.5rem;margin-top:.75rem}
.audit-commit{background:var(--card-2);border:1px solid var(--border);border-radius:6px;overflow:hidden}
.audit-commit[open]{border-color:var(--accent)}
.audit-commit-head{display:flex;flex-wrap:wrap;gap:.6rem;align-items:center;padding:.5rem .75rem;cursor:pointer;list-style:none}
.audit-commit-head::-webkit-details-marker{display:none}
.audit-commit-head:hover{background:var(--card)}
.audit-hash{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;color:var(--accent);font-size:.8rem;font-weight:600}
.audit-author{color:var(--fg);font-size:.8rem;font-weight:600}
.audit-date{color:var(--muted);font-size:.78rem;font-variant-numeric:tabular-nums}
.audit-stats{margin-left:auto;color:var(--muted);font-size:.78rem;font-variant-numeric:tabular-nums}
.audit-message{margin:0;padding:.5rem .75rem;background:var(--bg);border-top:1px solid var(--border);border-bottom:1px solid var(--border);font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:.78rem;line-height:1.5;white-space:pre-wrap;word-break:break-word;color:var(--fg)}
.audit-diff-wrap{margin:0}
.audit-diff-head{padding:.3rem .75rem;background:#141a22;color:var(--muted);font-size:.7rem;text-transform:uppercase;letter-spacing:.1em;border-bottom:1px solid var(--border)}
.audit-diff-empty{padding:.5rem .75rem}
.audit-diff{margin:0;padding:.5rem .75rem;background:#0b0f14;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:.75rem;line-height:1.45;white-space:pre;overflow-x:auto;tab-size:2}
.audit-diff code{font-family:inherit;font-size:inherit;background:transparent}
.diff-file{color:#79c0ff;font-weight:600;display:block}
.diff-meta{color:var(--muted)}
.diff-path{color:#79c0ff;font-weight:600}
.diff-hunk{color:#d29922;background:rgba(210,153,34,.08);display:inline-block;width:100%}
.diff-add{color:#3fb950;background:rgba(63,185,80,.07)}
.diff-del{color:#f85149;background:rgba(248,81,73,.07)}
.diff-ctx{color:var(--fg);opacity:.85}
.xml-tag{color:#79c0ff}
.xml-name{color:#d2a8ff;font-weight:600}
.xml-attr-name{color:#d29922}
.xml-attr-val{color:#a5d6ff}
.xml-comment{color:var(--muted);font-style:italic}
.xml-decl{color:#d29922;font-style:italic}
@media(max-width:640px){.member-row{flex-direction:column;align-items:flex-start}.member-main{flex-direction:column;align-items:flex-start}.meta-box{width:100%}.dashboard-grid{grid-template-columns:1fr}.raw-env-grid{grid-template-columns:1fr}.audit-stats{margin-left:0}}
</style>`
