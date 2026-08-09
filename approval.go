package opnborg

import (
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// approval.go manages the security-approval ledger: a single on-disk SQLite
// database (approval.db, co-located with the backup store) that tracks every
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
	// _approvalDBName is the on-disk filename of the approval ledger, placed
	// at the root of the backup store. It is added to .gitignore so the
	// auto-commit loop never commits the ledger itself.
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
// commit is approved, and when it was approved.
type approvalState struct {
	approved      bool
	trueTimestamp time.Time
}

// approvalDB is the package-global ledger handle. It is opened once (from
// approvalDBOpen, called after gitInit at startup and lazily from the httpd
// approve handlers) and reused for the lifetime of the process. A dedicated
// mutex guards the open so the httpd goroutine and the backup workers never
// race on initialisation; all subsequent SQL access is serialised by the same
// mutex because SQLite (and the audit UI) does not benefit from concurrent
// writes here and the ledger is tiny.
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
	dbPath := filepath.Join(storePath, _approvalDBName)
	approvalDBMu.Lock()
	defer approvalDBMu.Unlock()
	if approvalDB != nil {
		if approvalDBAt == dbPath {
			return approvalDB, nil
		}
		_ = approvalDB.Close()
		approvalDB = nil
	}
	dsn := "file:" + dbPath + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)"
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

// approvalDBHandle returns the opened ledger handle, opening it on first use
// against the given store directory. It is safe to call concurrently.
func approvalDBHandle(storePath string) (*sql.DB, error) {
	dbPath := filepath.Join(storePath, _approvalDBName)
	approvalDBMu.Lock()
	h := approvalDB
	p := approvalDBAt
	approvalDBMu.Unlock()
	if h != nil && p == dbPath {
		return h, nil
	}
	return approvalDBOpen(storePath)
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
// and capped for ledger display. For Ollama-assisted messages this is the TLDR
// headline; for plain commits it is the static "opnborg auto update" subject.
func commitHeadline(msg string) string {
	for line := range strings.SplitSeq(msg, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			if len(line) > 160 {
				return line[:160]
			}
			return line
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
	_, err = db.Exec(`UPDATE approval SET approved = 1, true_timestamp = ?, source_ip = ?, x_forwarded_for = ?, remote_user = ? WHERE commit_hash = ?`,
		time.Now().UTC().Format(time.RFC3339), sourceIP, xForwardedFor, remoteUser, fullHash)
	return err
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
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	res, err := tx.Exec(`UPDATE approval SET approved = 1, true_timestamp = ?, source_ip = ?, x_forwarded_for = ?, remote_user = ? WHERE approved = 0`,
		ts, sourceIP, xForwardedFor, remoteUser)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// approvalGet returns the approval state for a single commit hash. A missing
// ledger, a missing row, or any error yields a zero (not-approved) state so
// the audit page always renders a valid approval box.
func approvalGet(config *OPNCall, fullHash string) approvalState {
	var st approvalState
	if config == nil || config.Path == "" {
		return st
	}
	db, err := approvalDBHandle(config.Path)
	if err != nil {
		return st
	}
	var approved int
	var ts string
	err = db.QueryRow(`SELECT approved, true_timestamp FROM approval WHERE commit_hash = ?`, fullHash).Scan(&approved, &ts)
	if err != nil {
		return st
	}
	st.approved = approved != 0
	if ts != "" {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			st.trueTimestamp = t
		}
	}
	return st
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
