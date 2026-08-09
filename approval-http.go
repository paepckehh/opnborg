package opnborg

import (
	"net/http"
	"strings"
)

// approval-http.go wires the security-approval ledger into the WebUI. It
// exposes two POST endpoints an operator invokes from the BorgAUDIT page:
//
//   - POST /approve?hash=<full-git-hash>&range=<range> marks a single tracked
//     commit as approved. The operator source identity (TCP source IP,
//     X-Forwarded-For reverse-proxy chain, Remote-User authenticated identity)
//     is captured from the request and recorded in the ledger alongside the
//     approval timestamp.
//   - POST /approve-all?range=<range> marks every pending tracked commit as
//     approved in one shot, recording the same operator identity on each row.
//
// Both endpoints redirect back to the audit page for the requested range so
// the operator sees the updated approval state immediately. The handlers are
// registered without the addSecurityHeader middleware (matching the /force
// handler) because they are mutating action endpoints, not page renders.

// getApproveHandler handles a single-commit approval toggle.
func getApproveHandler() http.Handler {
	h := func(r http.ResponseWriter, q *http.Request) {
		if q.Method != http.MethodPost && q.Method != http.MethodGet {
			http.Error(r, "Error: Method Not Allowed (405) ["+q.Method+"]", http.StatusMethodNotAllowed)
			return
		}
		hash := strings.TrimSpace(q.URL.Query().Get("hash"))
		if hash == "" || _cfg == nil {
			http.Redirect(r, q, auditRedirectTarget(q), http.StatusSeeOther)
			return
		}
		sourceIP, xff, remoteUser := approvalSourceFromRequest(q)
		if err := approvalApprove(_cfg, hash, sourceIP, xff, remoteUser); err != nil {
			displayChan <- []byte("[APPROVAL][APPROVE][FAIL][" + hash + "] " + err.Error())
		} else {
			displayChan <- []byte("[APPROVAL][APPROVE][OK][" + hash + "] by " + approvalSourceLabel(sourceIP, xff, remoteUser))
		}
		http.Redirect(r, q, auditRedirectTarget(q), http.StatusSeeOther)
	}
	return http.HandlerFunc(h)
}

// getApproveAllHandler handles the bulk approve-all action.
func getApproveAllHandler() http.Handler {
	h := func(r http.ResponseWriter, q *http.Request) {
		if q.Method != http.MethodPost && q.Method != http.MethodGet {
			http.Error(r, "Error: Method Not Allowed (405) ["+q.Method+"]", http.StatusMethodNotAllowed)
			return
		}
		if _cfg != nil {
			sourceIP, xff, remoteUser := approvalSourceFromRequest(q)
			n, err := approvalApproveAll(_cfg, sourceIP, xff, remoteUser)
			if err != nil {
				displayChan <- []byte("[APPROVAL][APPROVE-ALL][FAIL] " + err.Error())
			} else {
				displayChan <- []byte("[APPROVAL][APPROVE-ALL][OK][" + itoa(n) + "] by " + approvalSourceLabel(sourceIP, xff, remoteUser))
			}
		}
		http.Redirect(r, q, auditRedirectTarget(q), http.StatusSeeOther)
	}
	return http.HandlerFunc(h)
}

// auditRedirectTarget returns the audit page URL for the range carried by the
// request, defaulting to the canonical audit range when none (or an unknown
// one) is present so the redirect always lands on a valid page.
func auditRedirectTarget(q *http.Request) string {
	rng := auditRangeSlug(q.URL.Query().Get("range"))
	return "audit?range=" + rng
}

// approvalSourceLabel renders a compact one-line label of the operator source
// identity for the display log channel.
func approvalSourceLabel(sourceIP, xff, remoteUser string) string {
	var b strings.Builder
	b.WriteString(sourceIP)
	if xff != "" {
		b.WriteString(" xff=")
		b.WriteString(xff)
	}
	if remoteUser != "" {
		b.WriteString(" user=")
		b.WriteString(remoteUser)
	}
	return b.String()
}

// itoa is a local strconv.Itoa alias-free helper kept to avoid pulling an
// extra import into approval-http.go; the bulk-approve count is a small
// non-negative int64.
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
