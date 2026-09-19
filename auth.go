package opnborg

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"strconv"
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

// argon2id derivation parameters (fixed defaults, see README.md).
//
// NOTE on threads: the derivation intentionally runs single-lane (1). All
// historical releases derived with threads=1, and the OPN_AUTH_HASH format
// does not encode the KDF parameters, so raising the lane count now would
// silently invalidate every credential already armed in the field. Keep 1.
const (
	_authArgonTime    = uint32(8)
	_authArgonMemory  = uint32(64 * 1024)
	_authArgonThreads = uint8(1)
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

	// _authMaxPasswordLen bounds the password input before the expensive
	// Argon2id derivation runs. No legitimate admin password reaches this
	// size; the cap keeps a hostile client from feeding multi-megabyte form
	// values into the KDF (cost scales with input length).
	_authMaxPasswordLen = 1024
)

// _authDeriveSem bounds concurrent Argon2id derivations (login attempts and
// /auth-hash generator requests) to a single lane. The fixed-parameter
// derivation costs ~64 MiB of memory and roughly a second of CPU, so
// unbounded concurrent derivations would let any network client multiply
// that cost into memory exhaustion, and parallel login attempts would each
// derive before the shared lockout is armed, defeating the doubling backoff.
var _authDeriveSem = make(chan struct{}, 1)

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
	refHash   []byte    // decoded reference key (computed once at authInit)
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
		displayChan <- []byte("[AUTH][DISABLED][INVALID-" + _envAuthHash + "] expected a base64-encoded 64-byte key (argon2id, time=8,memory=65536,threads=1,keylen=64)")
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
	// Decode the reference key once here so the login path never has to
	// re-decode (and can derive the candidate outside the mutex).
	if ref, err := decodeHashHalf(hash); err == nil {
		auth.refHash = ref
	} else {
		auth.enabled = false
		auth.hash = ""
		auth.salt = nil
		displayChan <- []byte("[AUTH][DISABLED][INVALID-" + _envAuthHash + "] " + err.Error())
		return
	}
	displayChan <- []byte("[AUTH][ENABLED] admin mode credentials armed, monitoring mode active until login")
}

// validAuthHashFormat checks that the OPN_AUTH_HASH value is either a single
// base64-encoded 64-byte key, or the legacy "<base64-key>$<base64-key>"
// format (key encoded twice). Both forms must decode to keylen bytes.
func validAuthHashFormat(s string) bool {
	_, err := decodeHashHalf(s)
	return err == nil
}

// decodeHashHalf decodes and validates the base64 hash from OPN_AUTH_HASH.
// Accepts two formats for backward compatibility:
//   - Legacy: "<base64-key>$<base64-key>" — the second half must decode to
//     keylen (64) bytes.
//   - Current: a single base64-encoded key that must decode to keylen bytes.
func decodeHashHalf(s string) ([]byte, error) {
	if before, after, ok := strings.Cut(s, "$"); ok {
		// Legacy "<b64>$<b64>" format.
		_ = before
		if after == "" {
			return nil, errors.New("empty hash part")
		}
		raw, err := base64.StdEncoding.DecodeString(after)
		if err != nil {
			return nil, errors.New("hash part is not valid base64")
		}
		if len(raw) != int(_authArgonKeyLen) {
			return nil, errors.New("hash part must decode to 64 bytes (keylen=64)")
		}
		return raw, nil
	}
	// Current single-key format.
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, errors.New("hash is not valid base64")
	}
	if len(raw) != int(_authArgonKeyLen) {
		return nil, errors.New("hash must decode to 64 bytes (keylen=64)")
	}
	return raw, nil
}

// authArgonDerive runs the fixed-parameter Argon2id KDF (time=8,
// memory=64MiB, threads=1, keylen=64).
func authArgonDerive(password string, salt []byte) []byte {
	return argon2.IDKey([]byte(password), salt, _authArgonTime, _authArgonMemory, _authArgonThreads, _authArgonKeyLen)
}

// generateAuthCredentials derives OPN_AUTH_HASH and OPN_AUTH_SALT for a new
// admin password using the fixed Argon2id parameter set and a fresh 16-byte
// random salt. OPN_AUTH_HASH is a single base64-encoded Argon2id key (64
// bytes). decodeHashHalf still accepts the legacy "<base64>$<base64>" format
// (key encoded twice) for credentials already armed in the field.
func generateAuthCredentials(password string) (hashEnv, saltEnv string) {
	// single-lane derivation (see _authDeriveSem): the generator endpoint is
	// reachable without authentication, so concurrent requests must never
	// run side by side.
	_authDeriveSem <- struct{}{}
	defer func() { <-_authDeriveSem }()
	salt := make([]byte, _authArgonSaltLen)
	_, _ = rand.Read(salt)
	key := authArgonDerive(password, salt)
	return base64.StdEncoding.EncodeToString(key), base64.StdEncoding.EncodeToString(salt)
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
//
// The expensive Argon2id derivation (~seconds, 64 MiB) deliberately runs
// OUTSIDE auth.mu: every page render and the 1 Hz /auth/state poller take
// the same mutex, so deriving under the lock would freeze the whole WebUI
// for the duration of every login attempt. The state snapshot (salt,
// reference key, lockout) is taken under the lock, the candidate key is
// derived unlocked in a single lane (_authDeriveSem) with the lockout
// re-checked once the lane is acquired, and the compare + bookkeeping
// re-take the lock. Log lines are emitted only after the lock is released.
func authCheckPassword(password string) (token string, wait time.Duration, err error) {
	if len(password) > _authMaxPasswordLen {
		return "", 0, errors.New("password too long")
	}
	auth.mu.Lock()
	if !auth.enabled {
		auth.mu.Unlock()
		return "", 0, errors.New("admin authentication is not configured (set OPN_AUTH_HASH and OPN_AUTH_SALT)")
	}
	// global lockout: every session shares the penalty window
	if remain := time.Until(auth.lockUntil); remain > 0 {
		auth.mu.Unlock()
		return "", remain, errors.New("login locked, please wait")
	}
	// 6h idle reset of the global fail counter
	if !auth.lastFail.IsZero() && time.Since(auth.lastFail) >= _authFailWindow {
		auth.fails = 0
	}
	salt, refHash := auth.salt, auth.refHash
	if refHash == nil && auth.hash != "" {
		// State was armed outside authInit (e.g. a test helper): decode the
		// reference key here. This is a cheap base64 decode, not the
		// expensive Argon2 derivation, so keeping it under the lock is fine.
		refHash, _ = decodeHashHalf(auth.hash)
	}
	auth.mu.Unlock()

	// Single-lane derivation (see _authDeriveSem). Once the lane is acquired
	// the lockout is re-checked: parallel attempts queued behind a failed one
	// must be refused without burning a 64 MiB derivation each, otherwise a
	// burst of concurrent guesses would defeat the shared doubling backoff.
	_authDeriveSem <- struct{}{}
	auth.mu.Lock()
	if !auth.enabled {
		auth.mu.Unlock()
		<-_authDeriveSem
		return "", 0, errors.New("admin authentication is not configured (set OPN_AUTH_HASH and OPN_AUTH_SALT)")
	}
	if remain := time.Until(auth.lockUntil); remain > 0 {
		auth.mu.Unlock()
		<-_authDeriveSem
		return "", remain, errors.New("login locked, please wait")
	}
	auth.mu.Unlock()
	candidate := authArgonDerive(password, salt)
	<-_authDeriveSem

	auth.mu.Lock()
	if !auth.enabled {
		auth.mu.Unlock()
		return "", 0, errors.New("admin authentication is not configured (set OPN_AUTH_HASH and OPN_AUTH_SALT)")
	}
	// a concurrent failed attempt may have armed a longer lockout while this
	// derivation ran; honour it instead of counting this attempt on top.
	if remain := time.Until(auth.lockUntil); remain > 0 {
		auth.mu.Unlock()
		return "", remain, errors.New("login locked, please wait")
	}
	if subtle.ConstantTimeCompare(candidate, refHash) != 1 {
		auth.fails++
		auth.lastFail = time.Now()
		wait = authArmWaitLocked(auth.fails)
		auth.mu.Unlock()
		// The log line is sent AFTER releasing auth.mu: the display channel
		// has a small buffer drained by a single stdout writer, and sending
		// under the lock would block every authIsAdmin caller (i.e. every
		// page render and /auth/state poll) whenever stdout back-pressures.
		displayChan <- []byte("[AUTH][LOGIN][FAIL] failed attempts=" + strconv.Itoa(auth.fails) + " global lock=" + wait.String())
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
	adminEnabled.Store(true)
	auth.mu.Unlock()
	// sent outside the lock, mirroring the failure path above
	displayChan <- []byte("[AUTH][LOGIN][OK] admin mode session unlocked (ttl " + _authSessionTTL.String() + ")")
	return token, 0, nil
}

// authArmWait computes the global lockout for the n-th consecutive failure:
// 10s, 20s, 40s, ... (doubling). The shift is capped so an implausible fail
// count can never overflow the duration into a negative (past-oriented)
// deadline, which would silently disable the lockout.
func authArmWait(fails int) time.Duration {
	if fails < 1 {
		return 0
	}
	shift := min(fails-1, 20)
	return time.Duration(_authBaseWaitSeconds<<shift) * time.Second
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
	remain := max(time.Until(auth.lockUntil), 0)
	return remain, auth.fails
}

// authLogout revokes the session carried by the given token. When no
// sessions remain the atomic adminEnabled flag is cleared so the render
// path (greyed-out download buttons) reflects the monitoring-only state.
func authLogout(token string) {
	auth.mu.Lock()
	delete(auth.sessions, token)
	// mirror while still holding the lock so a login minting a fresh session
	// between unlock and the Store can never leave a live admin session with
	// adminEnabled == false (greyed-out download buttons for an admin).
	adminEnabled.Store(len(auth.sessions) > 0)
	auth.mu.Unlock()
}

// authSessionAdmin reports whether the request's session cookie carries a
// live admin-mode session and refreshes the sliding expiry window. It also
// mirrors the result into the atomic adminEnabled flag so the render path
// (greyed-out download buttons) can read it without a request handle.
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
		// mirror the surviving-session count so an expired last session
		// clears adminEnabled and the render path stops offering unlocked
		// download buttons that /files/ would correctly refuse.
		adminEnabled.Store(len(auth.sessions) > 0)
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
