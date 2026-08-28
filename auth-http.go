package opnborg

import (
	"html"
	"net/http"
	"strings"
)

// auth-http.go wires the two-mode access model into the WebUI:
//
//   - POST /auth/login?next=<path> verifies the admin password (Argon2id)
//     and sets a HttpOnly SameSite=Strict session cookie. Failures arm a
//     GLOBAL lockout shared by every browser session (10s, 20s, 40s, ...
//     doubling per consecutive failure, 6h idle reset). The login dialog
//     polls /auth/state while the derivation runs to show the clock
//     standby animation and the live lock countdown.
//   - POST /auth/logout revokes the session and returns to monitoring mode.
//   - POST /auth/hash accepts a bootstrap password and returns the
//     OPN_AUTH_HASH / OPN_AUTH_SALT env lines to copy into the daemon
//     environment (the authentication tile on the config dashboard). It is
//     disabled once real credentials are armed, so it can never be abused
//     to overwrite an existing operator password.
//   - GET /auth/state exposes the mode + remaining lock for JS polling.
//
// requireAdmin gates mutating action endpoints (/force, /approve,
// /approve-all) and the /files/ static file server: when credentials are
// armed and the request carries no live admin session, the request is
// redirected to the index with ?auth=locked so the operator is told to
// authenticate first. Page-render routes (/, /config, /audit, /progress)
// are NOT gated: they are always viewable; sensitive sub-features (diffs,
// download buttons, approve controls) are greyed out in the render path
// when no admin session is present. When no credentials are configured
// the middleware is a pass-through.

// requireAdmin wraps a mutating action handler so only authenticated admin
// sessions reach it. When credentials are armed and the request lacks a
// live admin session the client is redirected to the index with
// ?auth=locked. When credentials are not configured the middleware is a
// pass-through (monitoring-only mode has no login to enforce).
func requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authCredentialsEnabled() {
			next.ServeHTTP(w, r)
			return
		}
		if authIsAdmin(r) {
			next.ServeHTTP(w, r)
			return
		}
		http.Redirect(w, r, "./?auth=locked", http.StatusSeeOther)
	})
}

// requireAdminFiles wraps the /files/ static file server so config-file
// downloads (current.xml / archive) are admin-only when credentials are
// armed. When credentials are not configured the middleware is a
// pass-through.
func requireAdminFiles(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authCredentialsEnabled() {
			next.ServeHTTP(w, r)
			return
		}
		if authIsAdmin(r) {
			next.ServeHTTP(w, r)
			return
		}
		http.Redirect(w, r, "config?auth=locked", http.StatusSeeOther)
	})
}

// getLoginHandler processes password submissions.
func getLoginHandler() http.Handler {
	h := func(r http.ResponseWriter, q *http.Request) {
		if q.Method != http.MethodPost {
			http.Error(r, "Error: Method Not Allowed (405) ["+q.Method+"]", http.StatusMethodNotAllowed)
			return
		}
		next := sanitizeAuthNext(q.URL.Query().Get("next"))
		if err := q.ParseForm(); err != nil {
			http.Error(r, "Error: Bad Request (400)", http.StatusBadRequest)
			return
		}
		password := q.FormValue("password")
		token, wait, err := authCheckPassword(password)
		if err != nil {
			displayChan <- []byte("[AUTH][LOGIN][DENIED] wait=" + wait.String())
			http.Redirect(r, q, "./?auth=fail&wait="+authWaitSeconds(wait)+"&next="+next, http.StatusSeeOther)
			return
		}
		http.SetCookie(r, &http.Cookie{
			Name:     _authSessionCookie,
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   int(_authSessionTTL.Seconds()),
		})
		http.Redirect(r, q, "./"+next, http.StatusSeeOther)
	}
	return http.HandlerFunc(h)
}

// getLogoutHandler revokes the session and drops the cookie.
func getLogoutHandler() http.Handler {
	h := func(r http.ResponseWriter, q *http.Request) {
		if q.Method != http.MethodPost {
			http.Error(r, "Error: Method Not Allowed (405) ["+q.Method+"]", http.StatusMethodNotAllowed)
			return
		}
		authLogout(authTokenFrom(q))
		http.SetCookie(r, &http.Cookie{
			Name:     _authSessionCookie,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			MaxAge:   -1,
		})
		http.Redirect(r, q, "./", http.StatusSeeOther)
	}
	return http.HandlerFunc(h)
}

// getAuthStateHandler streams the current mode + lock state as JSON for the
// login dialog poller and the nav-bar countdown.
func getAuthStateHandler() http.Handler {
	h := func(r http.ResponseWriter, q *http.Request) {
		r.Header().Set(_ctype, "application/json")
		lock, fails := authRemainingLock()
		mode := "monitoring"
		if authIsAdmin(q) {
			mode = "admin"
		}
		_, _ = r.Write([]byte(`{"mode":"` + mode +
			`","credentials":` + boolJSON(authCredentialsEnabled()) +
			`,"lock_seconds":` + itoa(int64(lock.Seconds())) +
			`,"fails":` + itoa(int64(fails)) + `}`))
	}
	return http.HandlerFunc(h)
}

// getAuthHashHandler renders the credential-generator bootstrap page. It is
// only armed while NO valid credentials are configured; once OPN_AUTH_HASH
// / OPN_AUTH_SALT are live the endpoint refuses to run so it cannot be used
// to replace an existing operator password.
func getAuthHashHandler() http.Handler {
	h := func(r http.ResponseWriter, q *http.Request) {
		r = headHTML(r)
		if !authCredentialsEnabled() {
			switch q.Method {
			case http.MethodGet:
				writeTransportCompressedPage(getAuthHashHTML("", "", ""), r, q, false)
			case http.MethodPost:
				if err := q.ParseForm(); err != nil {
					http.Error(r, "Error: Bad Request (400)", http.StatusBadRequest)
					return
				}
				pw := q.FormValue("password")
				pw2 := q.FormValue("password2")
				if pw == "" || len(pw) < 5 {
					writeTransportCompressedPage(getAuthHashHTML("", "", "password too short (minimum 5 characters)"), r, q, false)
					return
				}
				if pw != pw2 {
					writeTransportCompressedPage(getAuthHashHTML("", "", "passwords do not match"), r, q, false)
					return
				}
				hashEnv, saltEnv := generateAuthCredentials(pw)
				writeTransportCompressedPage(getAuthHashHTML(hashEnv, saltEnv, ""), r, q, false)
			default:
				http.Error(r, "Error: Method Not Allowed (405) ["+q.Method+"]", http.StatusMethodNotAllowed)
			}
			return
		}
		// credentials armed: generator permanently closed
		http.Error(r, "Error: Forbidden (403): credentials already configured, regenerate on a fresh instance (unset OPN_AUTH_HASH first)", http.StatusForbidden)
	}
	return http.HandlerFunc(h)
}

// sanitizeAuthNext constrains the post-login redirect target to a small
// allow-list of relative pages so an open-redirect cannot smuggle operators
// off the WebUI (or onto //evil.example style scheme-relative URLs).
func sanitizeAuthNext(next string) string {
	switch strings.TrimSuffix(strings.TrimPrefix(next, "?"), "/") {
	case "config":
		return "config"
	case "audit":
		return "audit"
	default:
		return ""
	}
}

// authWaitSeconds renders a wait duration as whole seconds for the redirect
// query string (minimum 1 so the dialog always shows a live countdown).
func authWaitSeconds(d interface{ Seconds() float64 }) string {
	s := int(d.Seconds())
	if s < 1 {
		s = 1
	}
	return itoa(int64(s))
}

// boolJSON renders a bool as a JSON literal.
func boolJSON(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// _pwQualityJS is a pure client-side, offline, local-only password quality
// checker. It shows a live quality bar (very-weak → weak → fair → good →
// strong) as the operator types, but never enforces a minimum beyond the 5
// character floor enforced by the server-side handler and the HTML
// minlength attribute. No network calls, no dictionaries fetched, no
// external dependencies — everything is computed in the browser from the
// typed string alone.
const _pwQualityJS = `<script>
function pwScore(pw){
  if(pw.length===0) return {cls:'pw-empty',label:'enter a password (minimum 5 characters)'};
  var s=0;
  if(pw.length>=5) s+=1;
  if(pw.length>=8) s+=1;
  if(pw.length>=12) s+=1;
  if(pw.length>=16) s+=1;
  var lower=pw.match(/[a-z]/),upper=pw.match(/[A-Z]/),digit=pw.match(/[0-9]/),special=pw.match(/[^a-zA-Z0-9]/);
  var variety=(lower?1:0)+(upper?1:0)+(digit?1:0)+(special?1:0);
  if(variety>=2) s+=1;
  if(variety>=3) s+=1;
  if(variety>=4) s+=1;
  var repeats=0;
  for(var i=0;i<pw.length-1;i++){if(pw[i]===pw[i+1])repeats++;}
  if(repeats>2&&pw.length<10) s-=1;
  if(pw.length<5) return {cls:'pw-very-weak',label:'too short (minimum 5 characters required)'};
  if(s<=2) return {cls:'pw-very-weak',label:'very weak password'};
  if(s<=4) return {cls:'pw-weak',label:'weak password'};
  if(s<=6) return {cls:'pw-fair',label:'fair password'};
  if(s<=8) return {cls:'pw-good',label:'good password'};
  return {cls:'pw-strong',label:'strong password'};
}
function updatePwQuality(){
  var pw=document.getElementById('pw1').value;
  var bar=document.getElementById('pw-bar');
  var lbl=document.getElementById('pw-label');
  var r=pwScore(pw);
  bar.className='auth-pw-bar-fill '+r.cls;
  lbl.textContent=r.label;
  updatePwMatch();
}
function updatePwMatch(){
  var p1=document.getElementById('pw1').value;
  var p2=document.getElementById('pw2').value;
  var m=document.getElementById('pw-match');
  if(p2.length===0){m.textContent='';m.className='auth-pw-match';return;}
  if(p1===p2){m.textContent='passwords match';m.className='auth-pw-match match-yes';}
  else{m.textContent='passwords do not match';m.className='auth-pw-match match-no';}
}
document.addEventListener('DOMContentLoaded',function(){
  var f=document.getElementById('auth-gen-form');
  if(f)f.addEventListener('submit',function(e){
    var p1=document.getElementById('pw1').value;
    if(p1.length<5){e.preventDefault();var lbl=document.getElementById('pw-label');lbl.textContent='too short (minimum 5 characters required)';var bar=document.getElementById('pw-bar');bar.className='auth-pw-bar-fill pw-very-weak';}
  });
});
</script>`

// getAuthHashHTML renders the bootstrap credential generator page. When
// hashEnv/saltEnv are non-empty the page shows the two env lines to copy;
// otherwise it shows the password form with a live password quality bar.
// The quality checker is pure client-side JS (offline, local-only) — it
// shows progress but never enforces a minimum beyond 5 characters.
func getAuthHashHTML(hashEnv, saltEnv, errText string) string {
	var s strings.Builder
	s.WriteString(_htmlStart)
	s.WriteString(_headStatic)
	s.WriteString(_bodyStart)
	s.WriteString(_bodyHead)
	s.WriteString("<nav><a href=\"./\"><button>[ &larr; Hive Index ]</button></a><a href=\"config\"><button>[ Config Dashboard ]</button></a></nav>")
	s.WriteString("<div class=\"dashboard auth-gen\">")
	s.WriteString("<h2>Authentication &middot; Credential Generator</h2>")
	if errText != "" {
		s.WriteString("<div class=\"auth-gen-err\">" + html.EscapeString(errText) + "</div>")
	}
	if hashEnv != "" {
		s.WriteString("<p class=\"cfg-intro\">Derivation complete (Argon2id, time=8, memory=64 MiB, threads=4, keylen=64). " +
			"Add both environment variables to your opnborg environment (e.g. your <code>.env</code> file or systemd unit) and restart the daemon to arm admin mode:</p>")
		s.WriteString("<div class=\"auth-env-line\"><span class=\"raw-env-name\">" + _envAuthHash + "=</span><code class=\"auth-env-code\">" + html.EscapeString(hashEnv) + "</code></div>")
		s.WriteString("<div class=\"auth-env-line\"><span class=\"raw-env-name\">" + _envAuthSalt + "=</span><code class=\"auth-env-code\">" + html.EscapeString(saltEnv) + "</code></div>")
		s.WriteString("<p class=\"cfg-intro\">Keep both values secret. After the restart the nav-bar [ Authenticate ] button unlocks admin mode with the password you entered above.</p>")
	} else {
		s.WriteString("<p class=\"cfg-intro\">Enter the admin password you want to use. opnborg derives the two environment variables " +
			"<code>" + _envAuthHash + "</code> and <code>" + _envAuthSalt + "</code> for you (Argon2id, time=8, memory=64 MiB, threads=4, keylen=64). " +
			"The password itself is never stored; only the derived hash is displayed once for you to copy into the environment, then restart opnborg.</p>")
		s.WriteString("<form class=\"auth-gen-form\" method=\"post\" action=\"auth-hash\" id=\"auth-gen-form\">")
		s.WriteString("<div class=\"auth-gen-field-label\">Password</div>")
		s.WriteString("<input class=\"auth-input\" type=\"password\" name=\"password\" id=\"pw1\" placeholder=\"admin password\" minlength=\"5\" required autocomplete=\"new-password\" oninput=\"updatePwQuality()\">")
		s.WriteString("<div class=\"auth-pw-strength\">")
		s.WriteString("<div class=\"auth-pw-bar-track\"><div class=\"auth-pw-bar-fill pw-empty\" id=\"pw-bar\"></div></div>")
		s.WriteString("<div class=\"auth-pw-label\" id=\"pw-label\">enter a password (minimum 5 characters)</div>")
		s.WriteString("</div>")
		s.WriteString("<div class=\"auth-gen-field-label\">Repeat Password</div>")
		s.WriteString("<input class=\"auth-input auth-gen-form-pw2\" type=\"password\" name=\"password2\" id=\"pw2\" placeholder=\"repeat password\" minlength=\"5\" required autocomplete=\"new-password\" oninput=\"updatePwMatch()\">")
		s.WriteString("<div class=\"auth-pw-match\" id=\"pw-match\"></div>")
		s.WriteString("<button type=\"submit\" class=\"btn btn-force\" id=\"pw-submit\">[ Generate Env Vars ]</button>")
		s.WriteString("</form>")
		s.WriteString(_pwQualityJS)
	}
	s.WriteString("</div>")
	s.WriteString(_bodyFooter)
	s.WriteString(_bodyEnd)
	s.WriteString(_htmlEnd)
	return s.String()
}

// getBodyHead renders the app header for a live request: the application
// name, then on the right side (directly left of the version number) the
// WebUI mode indicator box (green = monitoring, red/yellow = admin) plus
// the [ Authenticate ] button (or [ Logout ] / lock countdown when armed),
// and finally the version pill.
func getBodyHead(q *http.Request) string {
	isAdmin := authIsAdmin(q)
	armed := authCredentialsEnabled()
	lock, fails := authRemainingLock()

	// mode indicator box
	modeBox := "<div class=\"mode-box mode-monitoring\" title=\"monitoring mode: no authentication required, config downloads and audit approvals are locked\">MONITORING</div>"
	if isAdmin {
		modeBox = "<div class=\"mode-box mode-admin\" title=\"admin mode: authenticated session, config downloads and audit approvals unlocked\">ADMIN</div>"
	} else if armed && lock > 0 {
		modeBox = "<div class=\"mode-box mode-lock\" title=\"login locked after failed attempts (shared across all sessions)\" id=\"auth-lock\" data-wait=\"" + itoa(int64(lock.Seconds())) + "\">LOCKED " + itoa(int64(lock.Seconds())) + "s</div>"
	}

	// auth action button
	authBtn := ""
	switch {
	case isAdmin:
		authBtn = "<form class=\"auth-nav-form\" method=\"post\" action=\"auth/logout\"><button type=\"submit\" class=\"auth-nav-btn auth-logout\" title=\"end the admin-mode session\">[ Logout ]</button></form>"
	case armed:
		if lock > 0 {
			authBtn = "<button type=\"button\" class=\"auth-nav-btn auth-locked\" disabled title=\"login locked, shared wait across all sessions\">[ Locked " + itoa(int64(lock.Seconds())) + "s ]</button>"
		} else {
			hint := ""
			if fails > 0 {
				hint = " (" + itoa(int64(fails)) + " failed)"
			}
			authBtn = "<button type=\"button\" class=\"auth-nav-btn\" onclick=\"openAuthDialog('')\" title=\"authenticate to unlock config downloads and audit approvals\">[ Authenticate ]</button>" + authDialog(hint)
		}
	default:
		authBtn = "<a href=\"auth-hash\"><button type=\"button\" class=\"auth-nav-btn auth-setup\" title=\"no credentials configured: create OPN_AUTH_HASH / OPN_AUTH_SALT\">[ Authenticate ]</button></a>"
	}
	// the dialog markup is only rendered when the login flow is actually
	// reachable: with no (or invalid) OPN_AUTH_* credentials there is nothing
	// to authenticate against, so every locked button points at the config
	// dashboard's authentication setup section instead.
	dialog := ""
	if !isAdmin && armed {
		dialog = authDialog("")
	}
	// the info dialog is always available in monitoring mode so every
	// greyed-out control can explain why it is locked and how to proceed.
	infoDialog := ""
	if !isAdmin {
		infoDialog = authInfoDialog(armed)
	}

	var s strings.Builder
	s.WriteString("<header class=\"app-header\"><h1>" + _app + "</h1>")
	s.WriteString("<div class=\"header-right\">")
	s.WriteString(modeBox)
	s.WriteString(authBtn)
	s.WriteString("<div class=\"semver\"><a href=\"https://paepcke.de/opnborg\">[ " + SemVer + " ]</a></div>")
	s.WriteString("</div></header>" + _lf)
	s.WriteString(dialog)
	s.WriteString(infoDialog)
	return s.String()
}

// authDialog renders the modal login dialog markup (hidden until opened via
// openAuthDialog). The derivation spinner is a rotating clock face; the
// dialog polls /auth/state to show the global lock countdown shared across
// all sessions. failWait (seconds) pre-arms the countdown after a failed
// redirect.
func authDialog(failHint string) string {
	pre := ""
	if failHint != "" {
		pre = "<div class=\"auth-dialog-fails\" id=\"auth-fails\">" + html.EscapeString(failHint) + "</div>"
	}
	return `<div class="auth-dialog-backdrop" id="auth-dialog" hidden>
<div class="auth-dialog">
<div class="auth-dialog-title">Admin Authentication</div>
<div class="auth-dialog-sub">monitoring mode only &#8212; authenticate to unlock config downloads and audit approvals</div>
<form id="auth-form" method="post" action="auth/login" onsubmit="return authSubmit()">
<input class="auth-input" id="auth-password" type="password" name="password" placeholder="admin password" autofocus autocomplete="current-password">
` + pre + `
<div class="auth-dialog-err" id="auth-err" hidden></div>
<div class="auth-wait" id="auth-wait" hidden><span class="auth-wait-clock" aria-hidden="true"></span><span class="auth-wait-clock-dial" aria-hidden="true"></span><span id="auth-wait-text"></span></div>
<button type="submit" class="btn btn-force auth-submit" id="auth-submit">[ Authenticate ]</button>
<button type="button" class="btn auth-cancel" onclick="closeAuthDialog()">[ Cancel ]</button>
</form>
<div class="auth-checking" id="auth-checking" hidden><div class="auth-clock"><div class="auth-clock-hand"></div></div><div class="auth-checking-text">verifying password &amp; deriving key (Argon2id)...</div></div>
</div>
</div>` + _lf
}

// authInfoDialog renders a modal dialog that explains why an action is
// locked in monitoring mode and how to unlock it. The content adapts to
// the current authentication state:
//   - when auth credentials are armed, the dialog explains how to login
//     via the nav bar [ Authenticate ] button, and offers a button that
//     opens the login dialog directly;
//   - when no credentials are configured, the dialog explains what admin
//     mode is, how to generate and configure OPN_AUTH_HASH / OPN_AUTH_SALT,
//     and links to the /auth-hash credential generator.
func authInfoDialog(armed bool) string {
	var body string
	if armed {
		body = `<div class="auth-info-step"><span class="auth-info-num">1</span><span>Click the <strong>[ Authenticate ]</strong> button in the top-right nav bar.</span></div>
<div class="auth-info-step"><span class="auth-info-num">2</span><span>Enter your admin password in the dialog that opens.</span></div>
<div class="auth-info-step"><span class="auth-info-num">3</span><span>After successful authentication, monitoring mode switches to <strong>admin mode</strong> and this action will be unlocked.</span></div>`
	} else {
		body = `<div class="auth-info-step"><span class="auth-info-num">1</span><span>Generate credentials: visit the <a href="auth-hash">/auth-hash</a> page to create an admin password and derive the <code>OPN_AUTH_HASH</code> and <code>OPN_AUTH_SALT</code> values.</span></div>
<div class="auth-info-step"><span class="auth-info-num">2</span><span>Set environment variables: configure <code>OPN_AUTH_HASH</code> and <code>OPN_AUTH_SALT</code> in your opnborg environment (e.g. in your <code>.env</code> file).</span></div>
<div class="auth-info-step"><span class="auth-info-num">3</span><span>Restart opnborg to apply the new configuration. The nav bar will then show the <strong>[ Authenticate ]</strong> login button.</span></div>`
	}
	actionBtn := ""
	if armed {
		actionBtn = `<button type="button" class="btn btn-force auth-info-action" onclick="closeAuthInfoDialog();openAuthDialog('')">[ Authenticate Now ]</button>`
	} else {
		actionBtn = `<a href="auth-hash"><button type="button" class="btn btn-force auth-info-action">[ Go to Auth Setup ]</button></a>`
	}
	return `<div class="auth-dialog-backdrop" id="auth-info-dialog" hidden>
<div class="auth-dialog auth-info-dialog">
<div class="auth-dialog-title">Action Locked: Monitoring Mode</div>
<div class="auth-info-msg" id="auth-info-msg" hidden></div>
<div class="auth-info-steps">` + body + `</div>
<div class="auth-info-actions">` + actionBtn + `<button type="button" class="btn auth-cancel" onclick="closeAuthInfoDialog()">[ Close ]</button></div>
</div>
</div>` + _lf
}

// _authJS is the small browser helper shipped with every page that carries
// the nav-bar auth UI. It opens/closes the login dialog, shows the clock
// standby animation while the Argon2id derivation round-trips, and polls
// /auth/state to render the global lock countdown shared across all
// sessions. Injected into _head at Setup() time.
const _authJS = `<script>
function openAuthDialog(msg){var d=document.getElementById('auth-dialog');if(!d)return;d.hidden=false;var e=document.getElementById('auth-err');if(msg&&e){e.textContent=msg;e.hidden=false;}var p=document.getElementById('auth-password');if(p)p.focus();}
function closeAuthDialog(){var d=document.getElementById('auth-dialog');if(d)d.hidden=true;stopAuthPoll();}
function showAuthInfoDialog(msg){var d=document.getElementById('auth-info-dialog');if(!d)return;d.hidden=false;var m=document.getElementById('auth-info-msg');if(msg&&m){m.textContent=msg;m.hidden=false;}else if(m){m.hidden=true;}}
function closeAuthInfoDialog(){var d=document.getElementById('auth-info-dialog');if(d)d.hidden=true;}
function stopAuthPoll(){if(window._authPoll){clearInterval(window._authPoll);window._authPoll=null;}}
function authCheckStart(){
 var chk=document.getElementById('auth-checking');var f=document.getElementById('auth-form');
 if(chk)chk.hidden=false;if(f)f.style.opacity='.35';var s=document.getElementById('auth-submit');if(s)s.disabled=true;
 window._authTo=setTimeout(function(){var c=document.querySelector('.auth-dialog');if(c)c.innerHTML='<div class="auth-dialog-title">verifying ...</div><div class="auth-checking-text">Argon2id derivation in progress (this can take a few seconds) &#8212; the page reloads automatically</div>';},400);
 return true;}
function authPollLock(){
 var box=document.getElementById('auth-wait');var txt=document.getElementById('auth-wait-text');var sub=document.getElementById('auth-submit');var nb=document.getElementById('auth-lock');
 fetch('auth/state').then(function(r){return r.json()}).then(function(j){
  if(j.lock_seconds>0){
   if(box){box.hidden=false;var m=Math.floor(j.lock_seconds/60),s2=j.lock_seconds%60;var c=(m>0)?(m+'m '+s2+'s'):((j.lock_seconds>=10)?j.lock_seconds+' seconds':j.lock_seconds+' second');if(txt)txt.textContent='login locked for all sessions: '+c+' (attempt '+j.fails+')';}
   if(sub)sub.disabled=true;
   if(nb)nb.textContent='LOCK '+c;
  }else{
   if(box)box.hidden=true;if(sub)sub.disabled=false;if(nb)nb.textContent='ADMIN LOCKED';
  }
 }).catch(function(){});
}
document.addEventListener('DOMContentLoaded',function(){
 var f=document.getElementById('auth-form');
 if(f)f.addEventListener('submit',function(){authCheckStart();});
 var q=new URLSearchParams(window.location.search);
 if(q.get('auth')==='fail'){var w=parseInt(q.get('wait')||'0',10);openAuthDialog(w>0?('login locked for '+w+'s (shared wait for all sessions), attempt #'+(q.get('f')||'?')):'invalid password, please try again');}
 if(q.get('auth')==='locked'){showAuthInfoDialog('monitoring mode: action locked, please authenticate first');}
 stopAuthPoll();window._authPoll=setInterval(authPollLock,1000);authPollLock();
});
</script>`
