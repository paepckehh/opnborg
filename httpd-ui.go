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

	_bodyHead   = "<header class=\"app-header\"><h1>" + _app + "</h1></header>" + _lf
	_bodySemVer = "<div class=\"semver\">[ " + SemVer + " ]</div>" + _lf
	_bodyFooter = "<footer><div class=\"footer-links\"><a href=\"https://paepcke.de/opnborg\">" + _git + "</a><a href=\"https://infosec.exchange/@paepcke\">" + _social + "</a></div><div class=\"footer-sponsor\">SPONSORED-BY: <a href=\"https://pvz.digital\">pvz.digital</a> <a href=\"https://debitor.de\">debitor.de</a></div><div class=\"footer-tag\">RESISTANCE IS FUTILE. YOUR OPNSENSE WILL BE ASSIMILATED.</div></footer>" + _lf

	_forceInfo   = "<div class=\"force-info\"><h2>[ performing backup ]</h2><p>wait for redirect</p></div>"
	_forceButton = "<a href=\"./force\" class=\"btn btn-force\">[ Backup NOW ]</a>"

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
.semver{color:var(--muted);font-size:.8rem;margin:.5rem 0}
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
.backup-section{margin:1rem 0;padding:1rem;background:var(--card);border:1px solid var(--border);border-radius:8px}
.backup-section b{color:var(--accent)}
.btn-force{display:inline-block;margin:.5rem 0;padding:.5rem 1rem;background:var(--accent);color:#fff;border:none;border-radius:6px;cursor:pointer;text-decoration:none;font-weight:600}
.btn-force:hover{filter:brightness(1.1)}
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
.dash-muted{color:var(--muted);font-style:italic}
.dash-ok{color:var(--ok)}
.dash-warn{color:var(--warn)}
.dash-err{color:var(--err)}
@media(max-width:640px){.member-row{flex-direction:column;align-items:flex-start}.member-main{flex-direction:column;align-items:flex-start}.meta-box{width:100%}.dashboard-grid{grid-template-columns:1fr}}
</style>`
