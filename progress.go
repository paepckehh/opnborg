package opnborg

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// progress.go provides a live log-line capture buffer plus a small set of
// atomic counters that back the animated forced-backup progress dashboard.
//
// Every line the display engine writes to stdout is also tee'd into a fixed
// size ring buffer here. When the operator hits "[ Backup NOW ]" the /force
// handler arms a fresh forced pass (incrementing forceSeq) and returns the
// animated dashboard page. The page polls /progress which streams the captured
// log lines back to the browser so the operator watches the backup happen in
// real time, and redirects back to the hive view once the forced pass ends.
//
// The capture is package-global on purpose: the display engine, the main
// backup loop and the httpd handlers all live in different call stacks and
// need a shared handle. The ring buffer + seq counters are guarded by
// progressMu; the busy / passSeq / forceSeq atomics are lock-free.

// _progressCap is the maximum number of recent log lines kept in memory. It
// is deliberately generous so a full forced pass (which fires one line per
// hive member plus one per changed file) stays fully captured for the polling
// dashboard even on a large fleet, while bounding long-run memory use.
const _progressCap = 512

// progressLine is a single captured log entry.
type progressLine struct {
	Seq uint64 `json:"seq"`
	Msg string `json:"msg"`
	TS  int64  `json:"ts"` // unix milliseconds
}

var (
	progressMu    sync.Mutex
	progressRing  = make([]progressLine, 0, _progressCap)
	progressSeq   uint64 // monotonic per-line sequence (1..N)
	progressStart time.Time

	backupBusy atomic.Bool
	passSeq    atomic.Uint64 // incremented at the start of every backup pass
	forceSeq   atomic.Uint64 // incremented by every /force poke
)

// appendProgress captures a single display line into the ring buffer. It is
// called by the display engine (startLog) for every message it emits, so the
// dashboard sees the exact same stream the operator reads on stdout.
func appendProgress(msg []byte) {
	progressMu.Lock()
	defer progressMu.Unlock()
	progressSeq++
	if len(progressRing) < _progressCap {
		progressRing = append(progressRing, progressLine{
			Seq: progressSeq,
			Msg: string(msg),
			TS:  time.Now().UnixMilli(),
		})
		return
	}
	// ring is full: drop the oldest entry and append the newest. Re-slicing
	// the backing array [1:] leaves capacity at the tail, so the append does
	// not re-allocate; the slice simply slides forward within the same array.
	progressRing = append(progressRing[1:], progressLine{
		Seq: progressSeq,
		Msg: string(msg),
		TS:  time.Now().UnixMilli(),
	})
}

// beginBackupPass marks the start of a backup pass in the main srv loop. It
// records the wall-clock start (used for the dashboard timer) and bumps
// passSeq so a dashboard that armed a force can detect when its forced pass
// has actually started running.
func beginBackupPass() {
	progressStart = time.Now()
	passSeq.Add(1)
	backupBusy.Store(true)
}

// endBackupPass marks the end of the current backup pass.
func endBackupPass() {
	backupBusy.Store(false)
}

// bumpForceSeq arms a fresh forced pass and returns the new sequence value so
// the /force handler can embed it into the dashboard page.
func bumpForceSeq() uint64 {
	return forceSeq.Add(1)
}

// progressSnapshot is the JSON payload returned by the /progress endpoint.
type progressSnapshot struct {
	Busy     bool           `json:"busy"`
	Pass     uint64         `json:"pass"`
	Force    uint64         `json:"force"`
	Elapsed  int64          `json:"elapsed_ms"`
	Captured int            `json:"captured"`
	Lines    []progressLine `json:"lines"`
}

// getProgressHandler returns the JSON status + incremental log lines the
// dashboard polls. Clients pass ?since=N to receive only lines with seq > N
// (the dashboard tracks the highest seq it has rendered and sends it back).
func getProgressHandler() http.Handler {
	h := func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json;charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		since := uint64(0)
		if sv := req.URL.Query().Get("since"); sv != "" {
			if n, err := strconv.ParseUint(sv, 10, 64); err == nil {
				since = n
			}
		}

		var elapsed int64
		if backupBusy.Load() && !progressStart.IsZero() {
			elapsed = time.Since(progressStart).Milliseconds()
		}

		snap := progressSnapshot{
			Busy:    backupBusy.Load(),
			Pass:    passSeq.Load(),
			Force:   forceSeq.Load(),
			Elapsed: elapsed,
		}

		progressMu.Lock()
		snap.Captured = len(progressRing)
		for _, l := range progressRing {
			if l.Seq > since {
				snap.Lines = append(snap.Lines, l)
			}
		}
		progressMu.Unlock()

		if snap.Lines == nil {
			snap.Lines = []progressLine{}
		}
		body, _ := json.Marshal(snap)
		_, _ = w.Write(body)
	}
	return http.HandlerFunc(h)
}
