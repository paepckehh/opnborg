package opnborg

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/argon2"
)

// auth.go implements the two-mode WebUI access model:
//
//   - Monitoring mode (default): no authentication required, the hive status
//     dashboard is fully readable but config-file downloads (current.xml /
//     archive) and BorgAUDIT approval actions are locked.
//   - Admin mode: unlocked per browser session via the OPN_AUTH_HASH /
//     OPN_AUTH_SALT env credentials (Argon2id password hash) and unlocks the
//     config downloads plus the security-approval actions.
//
// The mode indicator in the top nav bar renders green for monitoring and
// red/yellow for admin. Login attempts are globally rate-limited with a
// doubling backoff shared across ALL browser sessions.

// argon2id derivation parameters (fixed defaults, see AGENTS.md).
const (
	_authArgonTime    = uint32(8)
	_authArgonMemory  = uint32(64 * 1024)
	_authArgonThreads = uint8(4) // documented default; IDKey derives its own lane count from 1..4 via runtime, fixed at call sites
	_authArgonKeyLen  = uint32(64)
	_authArgonSaltLen = 16

	// _authSessionTTL is the admin-mode session lifetime. Successful
	// re-login is required after the TTL expires.
	_authSessionTTL = 12 * time.Hour

	// _authSessionCookie is the browser cookie carrying the session token.
	_authSessionCookie = "opnborg_auth"

	// _authFailWindow is the idle window after which the global fail
	// counter auto-resets (absent a successful login or process restart).
	_authFailWindow = 6 * time.Hour

	// _authBaseWaitSeconds is the lockout wait for the FIRST failed
	// attempt; every further failure doubles it (10s, 20s, 40s, ...).
	_authBaseWaitSeconds = 10
)

// OPN_AUTH env var names (see README.md).
const (
	_envAuthHash = "OPN_AUTH_HASH"
	_envAuthSalt = "OPN_AUTH_SALT"
)

// authState is the process-global authentication state.
type authState struct {
	mu        sync.Mutex
	sessions  map[string]time.Time // token -> expiry (admin mode unlocked)
	enabled   bool                 // OPN_AUTH_HASH + OPN_AUTH_SALT configured
	hash      string
	salt      []byte
	fails     int       // global failed-login counter (all sessions)
	lockUntil time.Time // global lockout deadline (all sessions)
	lastFail  time.Time // idle window start for the 6h counter reset
}

var auth = &authState{sessions: make(map[string]time.Time)}

// adminEnabled reports whether the WebUI currently renders unlocked
// (admin-mode) controls. Cheap atomic read used on every render path.
var adminEnabled atomic.Bool

// authInit reads the OPN_AUTH_* credentials from the environment at startup
// and arms the login feature when both carry valid-looking values. Called
// once from Setup(). Invalid-looking values disable the feature (with a log
// line) rather than failing the daemon: an operator should never lose the
// backup pipeline over a WebUI convenience credential.
func authInit() {
	auth.mu.Lock()
	defer auth.mu.Unlock()
	auth.enabled = false
	auth.hash = ""
	auth.salt = nil
	auth.fails = 0
	auth.lockUntil = time.Time{}
	adminEnabled.Store(false)
	hash := strings.TrimSpace(os.Getenv(_envAuthHash))
	salt := strings.TrimSpace(os.Getenv(_envAuthSalt))
	if hash == "" || salt == "" {
		return
	}
	if !validAuthHashFormat(hash) {
		displayChan <- []byte("[AUTH][DISABLED][INVALID-" + _envAuthHash + "] expected '<base64-key>$<base64-key>' (argon2id, time=8,memory=65536,threads=4,keylen=64)")
		return
	}
	saltBytes, err := base64.StdEncoding.DecodeString(salt)
	if err != nil || len(saltBytes) < 8 {
		displayChan <- []byte("[AUTH][DISABLED][INVALID-" + _envAuthSalt + "] expected a base64 encoded random salt (>= 8 raw bytes)")
		return
	}
	auth.enabled = true
	auth.hash = hash
	auth.salt = saltBytes
	displayChan <- []byte("[AUTH][ENABLED] admin mode credentials armed, monitoring mode active until login")
}

// validAuthHashFormat checks the "<base64-key>$<base64-key>" credential
// shape: both halves must be valid base64 decoding to keylen bytes.
func validAuthHashFormat(s string) bool {
	_, err := decodeHashHalf(s)
	return err == nil
}

// decodeHashHalf decodes and validates the base64 hash half of the
// OPN_AUTH_HASH value ("first$second"): the second half must decode to
// exactly keylen (64) bytes.
func decodeHashHalf(s string) ([]byte, error) {
	_, h, _ := strings.Cut(s, "$")
	if h == "" {
		return nil, errors.New("empty hash part")
	}
	raw, err := base64.StdEncoding.DecodeString(h)
	if err != nil {
		return nil, errors.New("hash part is not valid base64")
	}
	if len(raw) != int(_authArgonKeyLen) {
		return nil, errors.New("hash part must decode to 64 bytes (keylen=64)")
	}
	return raw, nil
}

// authArgonDerive runs the fixed-parameter Argon2id KDF (time=8,
// memory=64MiB, threads=4, keylen=64).
func authArgonDerive(password string, salt []byte) []byte {
	return argon2.IDKey([]byte(password), salt, _authArgonTime, _authArgonMemory, 1, _authArgonKeyLen)
}

// generateAuthCredentials derives OPN_AUTH_HASH and OPN_AUTH_SALT for a new
// admin password using the fixed Argon2id parameter set and a fresh 16-byte
// random salt. The OPN_AUTH_HASH format is "<base64-key>$<base64-key>"
// (the key material encoded twice, separated by '$') so the credential is a
// single opaque env line.
func generateAuthCredentials(password string) (hashEnv, saltEnv string) {
	salt := make([]byte, _authArgonSaltLen)
	_, _ = rand.Read(salt)
	key := authArgonDerive(password, salt)
	enc := base64.StdEncoding.EncodeToString(key)
	return enc + "$" + enc, base64.StdEncoding.EncodeToString(salt)
}

// authCredentialsEnabled reports whether login is armed.
func authCredentialsEnabled() bool {
	auth.mu.Lock()
	defer auth.mu.Unlock()
	return auth.enabled
}

// authCheckPassword verifies a login attempt. On success it mints a session
// and returns its token. On failure it bumps the global fail counter and
// arms the shared lockout; the returned error carries the remaining wait so
// the login dialog can display it.
func authCheckPassword(password string) (token string, wait time.Duration, err error) {
	auth.mu.Lock()
	defer auth.mu.Unlock()
	if !auth.enabled {
		return "", 0, errors.New("admin authentication is not configured (set OPN_AUTH_HASH and OPN_AUTH_SALT)")
	}
	// global lockout: every session shares the penalty window
	if remain := time.Until(auth.lockUntil); remain > 0 {
		return "", remain, errors.New("login locked, please wait")
	}
	// 6h idle reset of the global fail counter
	if !auth.lockUntil.IsZero() || auth.fails > 0 {
		if !auth.lastFail.IsZero() && time.Since(auth.lastFail) >= _authFailWindow {
			auth.fails = 0
		}
	}
	refHash, decErr := decodeHashHalf(auth.hash)
	if decErr != nil {
		return "", 0, errors.New("admin credential hash is invalid")
	}
	candidate := authArgonDerive(password, auth.salt)
	if subtle.ConstantTimeCompare(candidate, refHash) != 1 {
		auth.fails++
		auth.lastFail = time.Now()
		wait = authArmWaitLocked(auth.fails)
		displayChan <- []byte("[AUTH][LOGIN][FAIL] failed attempts=" + itoa(int64(auth.fails)) + " global lock=" + wait.String())
		return "", wait, errors.New("invalid password")
	}
	// success: reset the fail counter, mint the session
	auth.fails = 0
	auth.lastFail = time.Time{}
	auth.lockUntil = time.Time{}
	tokenRaw := make([]byte, 32)
	_, _ = rand.Read(tokenRaw)
	token = base64.RawURLEncoding.EncodeToString(tokenRaw)
	auth.sessions[token] = time.Now().Add(_authSessionTTL)
	displayChan <- []byte("[AUTH][LOGIN][OK] admin mode session unlocked (ttl " + _authSessionTTL.String() + ")")
	return token, 0, nil
}

// authArmWait computes the global lockout for the n-th consecutive failure:
// 10s, 20s, 40s, ... (doubling).
func authArmWait(fails int) time.Duration {
	if fails < 1 {
		return 0
	}
	seconds := _authBaseWaitSeconds
	for i := 1; i < fails; i++ {
		seconds *= 2
	}
	return time.Duration(seconds) * time.Second
}

// authArmWaitLocked stores the lockout deadline for n accumulated failures.
func authArmWaitLocked(fails int) time.Duration {
	wait := authArmWait(fails)
	auth.lockUntil = time.Now().Add(wait)
	return wait
}

// authRemainingLock returns the remaining global lockout (zero when open)
// plus the current fail count for the nav-bar / dialog rendering.
func authRemainingLock() (time.Duration, int) {
	auth.mu.Lock()
	defer auth.mu.Unlock()
	remain := time.Until(auth.lockUntil)
	if remain < 0 {
		remain = 0
	}
	return remain, auth.fails
}

// authLogout revokes the session carried by the given token.
func authLogout(token string) {
	auth.mu.Lock()
	defer auth.mu.Unlock()
	delete(auth.sessions, token)
}

// authSessionAdmin reports whether the request's session cookie carries a
// live admin-mode session and refreshes the sliding expiry window.
func authSessionAdmin(token string) bool {
	if token == "" {
		return false
	}
	auth.mu.Lock()
	defer auth.mu.Unlock()
	exp, ok := auth.sessions[token]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(auth.sessions, token)
		return false
	}
	auth.sessions[token] = time.Now().Add(_authSessionTTL)
	return true
}

// authTokenFrom extracts the session cookie from a request.
func authTokenFrom(q *http.Request) string {
	if q == nil {
		return ""
	}
	c, err := q.Cookie(_authSessionCookie)
	if err != nil || c.Value == "" {
		return ""
	}
	return c.Value
}

// authIsAdmin is the single render-path gate: true when the request carries
// a live admin-mode session.
func authIsAdmin(q *http.Request) bool {
	return authSessionAdmin(authTokenFrom(q))
}
