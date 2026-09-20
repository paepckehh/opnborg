package opnborg

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"

	_ "modernc.org/sqlite"
)

// approval.go manages the security-approval ledger: a single on-disk SQLite
// database (approval.db, living in the dedicated .db directory inside the
// backup store) that tracks every
// opnborg-authored git commit whose Ollama security-impact tag is above
// "low"/"none"/"backup" (i.e. medium / high / critical). For each tracked
// commit the ledger records the full git hash, the security severity, the
// commit headline, the commit timestamp, and an approval state an operator can
// toggle from the BorgAUDIT WebUI page. Toggling a commit to approved records
// the wall-clock timestamp of the toggle together with the source IP address,
// the X-Forwarded-For reverse-proxy chain, and the Remote-User authenticated
// identity of the operator who made the change, so every approval carries a
// full audit trail of who acted and from where.
//
// The database uses SQLite STRICT mode so every column carries an explicit
// affix type and SQLite refuses to store a value whose type does not match the
// declared column type. The single "approval" table holds one row per tracked
// commit, keyed by the full git commit hash (the unique primary key).

const (
	// _approvalDBDir is the dedicated directory inside the backup store that
	// holds the security-approval ledger and nothing else: <storePath>/.db.
	// Everything inside it is gitignored (see _ignore in git.go) and
	// hardwire-excluded from commit staging (isApprovalDBPath in git.go): no
	// file below .db — the ledger database, its SQLite WAL sidecars
	// (approval.db-wal / approval.db-shm), or any stray file — may ever be
	// added, evaluated, or committed by the opnborg commit cycle, nor picked
	// up by a manual `git add .` thanks to the ".db/" .gitignore entry. A
	// legacy root-level ledger (approval.db at the store root, as written by
	// older releases) is migrated into .db at startup by approvalDBMigrate.
	_approvalDBDir = ".db"
	// _approvalDBName is the on-disk filename of the approval ledger, placed
	// inside the dedicated .db directory of the backup store. It is gitignored
	// (see _ignore in git.go): the ledger is a local-only runtime database and
	// must never be added, evaluated, or committed by the opnborg commit cycle.
	// The .gitignore carries a ".db/" entry plus the explicit approval.db /
	// approval.db-wal / approval.db-shm names so the main database, its SQLite
	// WAL sidecars, and every other file of the .db directory stay untracked
	// even for manual git commands. gitCommit additionally skips every
	// approval-ledger path at staging time so a ledger update can never enter
	// a commit even if a stale or hand-edited .gitignore failed to ignore it.
	_approvalDBName = "approval.db"
	// _approvalDriver is the database/sql driver name registered by
	// modernc.org/sqlite (a pure-Go, CGO-free SQLite implementation, so the
	// opnborg binary stays static-buildable with CGO_ENABLED=0 as the
	// Dockerfile and goreleaser both require).
	_approvalDriver = "sqlite"
)

// _approvalSchema is the STRICT table definition. STRICT mode enforces
// per-column type checking: every column carries an explicit affix type
// (TEXT / INTEGER) and SQLite rejects any row whose value type does not match
// the declared column type. The commit_hash column is the unique primary key:
// each tracked commit yields exactly one row, so re-tracking a commit that is
// already in the ledger is a safe no-op (INSERT OR IGNORE).
const _approvalSchema = `CREATE TABLE IF NOT EXISTS approval (
	commit_hash     TEXT    NOT NULL PRIMARY KEY,
	approved        INTEGER NOT NULL DEFAULT 0,
	true_timestamp  TEXT    NOT NULL DEFAULT '',
	source_ip       TEXT    NOT NULL DEFAULT '',
	x_forwarded_for TEXT    NOT NULL DEFAULT '',
	remote_user     TEXT    NOT NULL DEFAULT '',
	severity        TEXT    NOT NULL DEFAULT '',
	commit_subject  TEXT    NOT NULL DEFAULT '',
	committed_at    TEXT    NOT NULL DEFAULT ''
) STRICT;`

// approvalState is the minimal approval view the audit page needs: whether the
// commit is approved, when it was approved, and the operator source identity
// (source IP, X-Forwarded-For chain, Remote-User) recorded at approval time so
// the audit page can render a full who-approved-when box.
type approvalState struct {
	approved      bool
	trueTimestamp time.Time
	sourceIP      string
	xForwardedFor string
	remoteUser    string
}

// approvalDB is the package-global ledger handle. It is opened once (from
// approvalDBOpen, called after gitInit at startup and lazily from the httpd
// approve handlers) and reused for the lifetime of the process. A dedicated
// mutex guards the open so the httpd goroutine and the backup workers never
// race on initialisation or on a close-and-reopen. Concurrent SQL access is
// safe by construction, NOT by this mutex: the pool is capped at a single
// connection (SetMaxOpenConns(1)) with a busy timeout, serialising every
// statement. Raising that pool limit without revisiting this design would
// expose the ledger to SQLITE_BUSY-style write contention.
var (
	approvalDB   *sql.DB
	approvalDBMu sync.Mutex
	approvalDBAt string
)

// approvalDBOpen opens (or reuses) the approval ledger at the root of the
// backup store. It is idempotent: once opened the stored handle is returned
// unchanged for the rest of the process lifetime as long as the store path
// matches. The schema is created on first open. A failure to open is returned
// to the caller; callers in the hot path (gitCommit tracking) treat the error
// as non-fatal so a ledger problem never blocks a backup from being committed.
// The storePath argument is the backup store directory; the ledger file is
// placed at <storePath>/approval.db.
func approvalDBOpen(storePath string) (*sql.DB, error) {
	approvalDBMu.Lock()
	defer approvalDBMu.Unlock()
	return approvalDBOpenLocked(storePath)
}

// approvalDBHandle returns the opened ledger handle, opening it on first use
// against the given store directory. It is safe to call concurrently and
// never hands out a handle that another goroutine may close and replace: the
// open-and-reuse decision is made while holding approvalDBMu, so a concurrent
// reopen (path change or approvalClose) cannot invalidate a handle mid-use.
func approvalDBHandle(storePath string) (*sql.DB, error) {
	approvalDBMu.Lock()
	defer approvalDBMu.Unlock()
	// approvalDBOpenLocked already implements the whole reuse / path-mismatch
	// / reopen sequence, so this is a pure critical-section wrapper.
	return approvalDBOpenLocked(storePath)
}

// approvalDBPath returns the on-disk location of the approval ledger
// database inside the given backup store: <storePath>/.db/approval.db. The
// dedicated .db directory keeps every ledger file (database, WAL sidecars,
// anything else) out of the git worktree evaluation path; the single source
// of truth for the location so open, exists, and migration never drift apart.
func approvalDBPath(storePath string) string {
	return filepath.Join(storePath, _approvalDBDir, _approvalDBName)
}

// approvalDBMigrate relocates a legacy root-level approval ledger into the
// dedicated <storePath>/.db directory and guarantees the store root holds no
// approval-ledger leftovers afterwards. Older opnborg releases placed
// approval.db (plus its approval.db-wal / approval.db-shm WAL sidecars)
// directly at the store root; every startup moves those three files into
// .db so the root stays clean. When the .db target already exists the fresh
// .db copy wins and the stale root leftover is removed, so a partially
// migrated or hand-restored store converges to the canonical layout. A
// rename failure (e.g. cross-device store) falls back to copy-then-remove.
// The migration is idempotent: a store without root-level ledger files is a
// no-op.
func approvalDBMigrate(storePath string) error {
	if storePath == "" {
		return nil
	}
	var legacy bool
	for _, name := range _approvalLedgerNames {
		if _, err := os.Stat(filepath.Join(storePath, name)); err == nil {
			legacy = true
			break
		}
	}
	if !legacy {
		return nil
	}
	dir := filepath.Join(storePath, _approvalDBDir)
	if err := os.MkdirAll(dir, 0770); err != nil {
		return fmt.Errorf("approval db dir: %w", err)
	}
	for _, name := range _approvalLedgerNames {
		src := filepath.Join(storePath, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dst := filepath.Join(dir, name)
		if _, err := os.Stat(dst); err == nil {
			// The .db copy is the canonical one: drop the stale root leftover.
			if err := os.Remove(src); err != nil {
				return fmt.Errorf("cleanup legacy %s: %w", src, err)
			}
			continue
		}
		if err := os.Rename(src, dst); err == nil {
			continue
		}
		// Rename refused (e.g. cross-device store): copy then remove.
		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("read legacy %s: %w", src, err)
		}
		if err := os.WriteFile(dst, data, 0660); err != nil {
			return fmt.Errorf("write %s: %w", dst, err)
		}
		if err := os.Remove(src); err != nil {
			return fmt.Errorf("remove legacy %s: %w", src, err)
		}
	}
	return nil
}

// approvalDBOpenLocked is the core of approvalDBOpen; it assumes
// approvalDBMu is already held. approvalDBHandle uses it directly so the whole
// check-open-reuse sequence is one critical section.
func approvalDBOpenLocked(storePath string) (*sql.DB, error) {
	dbPath := approvalDBPath(storePath)
	if err := os.MkdirAll(filepath.Dir(dbPath), 0770); err != nil {
		return nil, fmt.Errorf("approval db dir: %w", err)
	}
	if approvalDB != nil {
		if approvalDBAt == dbPath {
			return approvalDB, nil
		}
		_ = approvalDB.Close()
		approvalDB = nil
		approvalDBAt = ""
	}
	dsn := "file:" + url.PathEscape(dbPath) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)"
	db, err := sql.Open(_approvalDriver, dsn)
	if err != nil {
		return nil, fmt.Errorf("approval db open: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(_approvalSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("approval db schema: %w", err)
	}
	approvalDB = db
	approvalDBAt = dbPath
	return db, nil
}

// approvalDBExists reports whether the on-disk approval ledger file already
// exists in the dedicated .db directory of the backup store. It is used by
// gitInit (after approvalDBMigrate has relocated any legacy root-level
// ledger) to detect a first-time ledger creation so the full storage-repo git
// history can be scanned once to backfill every pre-existing security-relevant
// commit into the freshly-created ledger. A stat error other than NotExist is
// treated as "does not exist" so a transient FS problem never blocks the open.
func approvalDBExists(storePath string) bool {
	_, err := os.Stat(approvalDBPath(storePath))
	return err == nil
}

// approvalBackfillFromHistory walks the entire storage-repo git log (no time
// window) and records every security-relevant commit (medium / high / critical
// Ollama security-impact tag) in the approval ledger. It is invoked once from
// gitInit when the ledger file is created for the first time, so commits that
// predate the ledger feature (or whose tag was authored before
// approvalTrackCommit was wired into gitCommit) are still surfaced for
// operator triage. Re-tracking a commit already present is a safe no-op
// (INSERT OR IGNORE) so the recorded approval state is never clobbered. The
// walk is bounded by _auditCap so a huge history never stalls startup. A
// missing repo, unborn HEAD, or un-openable ledger is a non-fatal no-op.
func approvalBackfillFromHistory(config *OPNCall) {
	if config == nil || config.Path == "" {
		return
	}
	repo, err := git.PlainOpen(config.Path)
	if err != nil {
		displayChan <- []byte("[APPROVAL][BACKFILL][SKIP] no repo: " + err.Error())
		return
	}
	head, err := repo.Head()
	if err != nil {
		// unborn HEAD: repo exists but has no commits yet; nothing to scan.
		return
	}
	iter, err := repo.Log(&git.LogOptions{From: head.Hash()})
	if err != nil {
		displayChan <- []byte("[APPROVAL][BACKFILL][FAIL] " + err.Error())
		return
	}
	defer iter.Close()
	var n int
	for {
		c, err := iter.Next()
		if err != nil {
			// Only io.EOF ends the log walk; any other error (corrupt object,
			// I/O failure) must surface, otherwise the backfill silently
			// truncates and security-relevant commits stay untracked.
			if !errors.Is(err, io.EOF) {
				displayChan <- []byte("[APPROVAL][BACKFILL][FAIL] " + err.Error())
				return
			}
			break
		}
		if n >= _auditCap {
			break
		}
		n++
		msg := strings.TrimSpace(c.Message)
		severity, _, _ := auditTag(msg)
		if !isSecurityRelevantTag(severity) {
			continue
		}
		approvalTrackCommit(config, c.Hash.String(), msg, c.Author.When)
	}
	displayChan <- []byte("[APPROVAL][BACKFILL][FINISH] scanned " + strconv.Itoa(n) + " commits")
}

// approvalClose closes the ledger. Used only by tests to release the file
// handle between runs on the same temp directory.
func approvalClose() {
	approvalDBMu.Lock()
	defer approvalDBMu.Unlock()
	if approvalDB != nil {
		_ = approvalDB.Close()
		approvalDB = nil
		approvalDBAt = ""
	}
}

// isSecurityRelevantTag reports whether a commit's security-impact tag is
// above the "low"/"none"/"backup" stage, i.e. the commit carries a medium /
// high / critical severity classification. Only such commits are tracked in
// the approval ledger: routine low-severity and Unifi autoBackup rotations
// (tagged "low, backup") and untagged commits do not need an operator
// approval.
func isSecurityRelevantTag(severity string) bool {
	switch severity {
	case "medium", "high", "critical":
		return true
	}
	return false
}

// approvalTrackCommit records a freshly-authored opnborg commit in the
// approval ledger when its security-impact tag is security-relevant (medium /
// high / critical). Commits whose tag is low, none, or a plain Unifi backup
// rotation are not tracked. Re-tracking a commit already in the ledger is a
// safe no-op (INSERT OR IGNORE) so the recorded approval state is never
// clobbered by a later re-scan. A nil or un-openable ledger is a non-fatal
// no-op: a ledger problem must never block a backup from being committed.
func approvalTrackCommit(config *OPNCall, fullHash, message string, committedAt time.Time) {
	if config == nil || config.Path == "" {
		return
	}
	severity, _, _ := auditTag(message)
	if !isSecurityRelevantTag(severity) {
		return
	}
	db, err := approvalDBHandle(config.Path)
	if err != nil {
		displayChan <- []byte("[APPROVAL][TRACK][FAIL] " + err.Error())
		return
	}
	subject := commitHeadline(message)
	_, err = db.Exec(`INSERT OR IGNORE INTO approval (commit_hash, approved, true_timestamp, source_ip, x_forwarded_for, remote_user, severity, commit_subject, committed_at) VALUES (?, 0, '', '', '', '', ?, ?, ?)`,
		fullHash, severity, subject, committedAt.UTC().Format(time.RFC3339))
	if err != nil {
		displayChan <- []byte("[APPROVAL][TRACK][FAIL] " + err.Error())
	}
}

// commitHeadline returns the first non-empty line of a commit message, trimmed
// and capped for ledger display. For Ollama-assisted messages this is the
// model's commit headline; for plain commits it is the static "opnborg auto
// update" subject.
func commitHeadline(msg string) string {
	for line := range strings.SplitSeq(msg, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return truncateUTF8(line, 160)
		}
	}
	return ""
}

// approvalApprove marks a single tracked commit as approved, recording the
// wall-clock timestamp of the toggle together with the operator's source IP,
// X-Forwarded-For chain, and Remote-User identity. It is idempotent: approving
// an already-approved commit refreshes the timestamp and source fields. A
// missing ledger or unknown hash is a non-fatal no-op.
func approvalApprove(config *OPNCall, fullHash, sourceIP, xForwardedFor, remoteUser string) error {
	if config == nil || config.Path == "" {
		return fmt.Errorf("approval: no store path")
	}
	db, err := approvalDBHandle(config.Path)
	if err != nil {
		return err
	}
	res, err := db.Exec(`UPDATE approval SET approved = 1, true_timestamp = ?, source_ip = ?, x_forwarded_for = ?, remote_user = ? WHERE commit_hash = ?`,
		time.Now().UTC().Format(time.RFC3339), sourceIP, xForwardedFor, remoteUser, fullHash)
	if err != nil {
		return err
	}
	// A 0-row update means the hash was never tracked in the ledger (a
	// low-severity commit, a typo, or a crafted value). Report it explicitly
	// so the caller does not log a false "approved" success.
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("commit %s not tracked in the approval ledger", fullHash)
	}
	return nil
}

// approvalApproveAll marks every pending (not-yet-approved) tracked commit as
// approved in a single transaction, recording the operator source identity on
// each row. It returns the number of rows newly approved.
func approvalApproveAll(config *OPNCall, sourceIP, xForwardedFor, remoteUser string) (int64, error) {
	if config == nil || config.Path == "" {
		return 0, fmt.Errorf("approval: no store path")
	}
	db, err := approvalDBHandle(config.Path)
	if err != nil {
		return 0, err
	}
	ts := time.Now().UTC().Format(time.RFC3339)
	// A single UPDATE is already atomic; the connection pool holds exactly
	// one connection, so an explicit transaction adds nothing here.
	res, err := db.Exec(`UPDATE approval SET approved = 1, true_timestamp = ?, source_ip = ?, x_forwarded_for = ?, remote_user = ? WHERE approved = 0`,
		ts, sourceIP, xForwardedFor, remoteUser)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// approvalGet returns the approval state for a single commit hash. A missing
// row yields a zero (not-approved) state. A ledger open failure or a query
// error other than "no rows" returns ok=false so the audit page can render a
// degraded approval box instead of a misleading unapproved one.
func approvalGet(config *OPNCall, fullHash string) (approvalState, bool) {
	var st approvalState
	if config == nil || config.Path == "" {
		return st, false
	}
	db, err := approvalDBHandle(config.Path)
	if err != nil {
		return st, false
	}
	var approved int
	var ts, sourceIP, xff, remoteUser string
	err = db.QueryRow(`SELECT approved, true_timestamp, source_ip, x_forwarded_for, remote_user FROM approval WHERE commit_hash = ?`, fullHash).Scan(&approved, &ts, &sourceIP, &xff, &remoteUser)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return st, true
		}
		return st, false
	}
	st.approved = approved != 0
	st.sourceIP = sourceIP
	st.xForwardedFor = xff
	st.remoteUser = remoteUser
	if ts != "" {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			st.trueTimestamp = t
		}
	}
	return st, true
}

// approvalPendingCount returns the number of tracked commits not yet
// approved. It powers the "approve all" button label so an operator can see at
// a glance how many approvals are outstanding. A missing ledger yields 0.
func approvalPendingCount(config *OPNCall) int {
	if config == nil || config.Path == "" {
		return 0
	}
	db, err := approvalDBHandle(config.Path)
	if err != nil {
		return 0
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM approval WHERE approved = 0`).Scan(&n)
	return n
}

// syncAuditCommitsToLedger ensures every security-relevant commit in the
// displayed audit window is tracked in the approval ledger. Historical commits
// that predate the ledger feature (or whose Ollama security-impact tag was
// authored before approvalTrackCommit was wired into gitCommit) would otherwise
// render an approve button on the audit page but never appear in the ledger,
// so approvalPendingCount (and the approve-all button label) would report 0
// pending even though the page visibly shows unapproved medium/high/critical
// commits. Re-tracking a commit already in the ledger is a safe no-op
// (INSERT OR IGNORE) so the recorded approval state is never clobbered.
func syncAuditCommitsToLedger(config *OPNCall, commits []auditCommit) {
	if config == nil || config.Path == "" {
		return
	}
	for _, c := range commits {
		severity, _, _ := auditTag(c.message)
		if !isSecurityRelevantTag(severity) || c.fullHash == "" {
			continue
		}
		approvalTrackCommit(config, c.fullHash, c.message, c.when)
	}
}

// approvalSourceFromRequest extracts the operator identity from an HTTP
// request: the direct TCP source IP (RemoteAddr, stripped to host:port's
// host part), the X-Forwarded-For header (the reverse-proxy chain, verbatim),
// and the Remote-User header (the authenticated identity a reverse proxy
// injected). All three are recorded on every approval so the ledger carries a
// full who-acted-from-where trail regardless of whether opnborg sits behind a
// reverse proxy.
func approvalSourceFromRequest(r *http.Request) (sourceIP, xForwardedFor, remoteUser string) {
	sourceIP = strings.TrimSpace(r.RemoteAddr)
	if host, _, err := net.SplitHostPort(sourceIP); err == nil {
		sourceIP = host
	}
	xForwardedFor = strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	remoteUser = strings.TrimSpace(r.Header.Get("Remote-User"))
	return sourceIP, xForwardedFor, remoteUser
}
