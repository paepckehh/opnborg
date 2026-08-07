package opnborg

import (
	"errors"
	"html"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// audit.go serves the "BorgAUDIT" WebUI tile and the per-range audit pages that
// render the storage repo git commit history (commit metadata + full unified
// diff per commit) for the last 24 hours / 7 days / 1 month. The audit tile on
// the index page exposes three buttons, each linking to a dedicated page so an
// operator can review recent backup-change history at a glance, with syntax
// highlighting for the diff details.

const (
	// _auditCap bounds the number of commits a single audit page walks and
	// renders so a huge history within the window never stalls the page.
	_auditCap = 250
	// _auditDiffCap bounds the rendered diff payload per single commit so a
	// full-config XML rotation does not blow up the page payload. The diff is
	// truncated with a visible marker when it overflows.
	_auditDiffCap = 512 * 1024
)

// _auditRanges maps the URL range slug to the time window it represents. The
// slug is the single query parameter the audit handler accepts (?range=24h).
var _auditRanges = map[string]time.Duration{
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"1m":  30 * 24 * time.Hour,
}

// _auditRangeLabels is the human-readable label shown in the page heading and
// the tile button text for each range slug. It is also the fallback order in
// which the tile buttons are emitted.
var _auditRangeLabels = []struct {
	slug  string
	label string
}{
	{"24h", "24 Hours"},
	{"7d", "7 Days"},
	{"1m", "1 Month"},
}

// _auditDefaultRange is the range served when the request carries no (or an
// unknown) ?range= parameter.
const _auditDefaultRange = "24h"

// getAuditTile renders the "BorgAUDIT" tile for the index page: a short
// heading and the three range buttons, each linking to the dedicated audit
// page for that window. The tile is only emitted when git storage management
// is enabled (OPN_GIT_ENABLE): without a git repo there is no commit history
// to audit.
func getAuditTile() string {
	if _cfg == nil || !_cfg.Git.Enable {
		return _empty
	}
	var s strings.Builder
	s.WriteString("<div class=\"backup-section audit-tile\"><b>BorgAUDIT</b> <span class=\"member-meta\">Module:Git:CommitHistory [ Review recent backup changes ]</span>")
	s.WriteString("<div class=\"tile-actions\">")
	for _, r := range _auditRangeLabels {
		s.WriteString("<a href=\"audit?range=")
		s.WriteString(r.slug)
		s.WriteString("\" class=\"btn btn-force\" target=\"_blank\">[ ")
		s.WriteString(r.label)
		s.WriteString(" ]</a>")
	}
	s.WriteString("</div>")
	s.WriteString("</div>")
	return s.String()
}

// getAuditHandler serves the audit page. It accepts a single ?range= query
// parameter (24h / 7d / 1m) and renders the commit history for that window.
// An unknown or missing range falls back to the default. The page is a
// standalone document (own head/nav/footer) so it can be opened in a tab.
func getAuditHandler() http.Handler {
	h := func(r http.ResponseWriter, q *http.Request) {
		r = headHTML(r)
		switch q.Method {
		case "GET":
			writeTransportCompressedPage(getAuditHTML(q.URL.Query().Get("range")), r, q, true)
		default:
			http.Error(r, "Error: Method Not Allowed (405) ["+q.Method+"]", http.StatusMethodNotAllowed)
		}
	}
	return http.HandlerFunc(h)
}

// getAuditHTML assembles the full audit page document for the given range
// slug (validated + defaulted via auditRangeSlug).
func getAuditHTML(rangeParam string) string {
	rangeSlug := auditRangeSlug(rangeParam)
	var s strings.Builder
	s.WriteString(_htmlStart)
	s.WriteString(_headStatic)
	s.WriteString(_bodyStart)
	s.WriteString(_bodyHead)
	s.WriteString(getAuditNavi(rangeSlug))
	s.WriteString(renderAuditPage(_cfg, rangeSlug))
	s.WriteString(_bodyFooter)
	s.WriteString(_bodyEnd)
	s.WriteString(_htmlEnd)
	return s.String()
}

// auditRangeSlug validates the incoming ?range= value against the known set
// and returns the default when it is missing or unknown.
func auditRangeSlug(v string) string {
	if _, ok := _auditRanges[v]; ok {
		return v
	}
	return _auditDefaultRange
}

// getAuditNavi is the top navigation for the audit page. It links back to the
// hive index, exposes the same external monitoring links as the index page,
// and renders the three range buttons with the active one highlighted.
func getAuditNavi(active string) string {
	var s strings.Builder
	s.WriteString("<nav>")
	s.WriteString("<a href=\"./\"><button>[ &larr; Hive Index ]</button></a>")
	for _, r := range _auditRangeLabels {
		cls := ""
		if r.slug == active {
			cls = " class=\"audit-active\""
		}
		s.WriteString("<a href=\"audit?range=")
		s.WriteString(r.slug)
		s.WriteString("\"><button")
		s.WriteString(cls)
		s.WriteString(">[ ")
		s.WriteString(r.label)
		s.WriteString(" ]</button></a>")
	}
	s.WriteString(getNavi())
	s.WriteString("</nav>")
	return s.String()
}

// renderAuditPage builds the audit body: a summary header (range, commit
// count, window boundaries), then the per-commit list. A nil config (httpd
// not armed yet, e.g. in tests) collapses to a placeholder. When git is
// disabled the page reports that the feature needs OPN_GIT_ENABLE.
func renderAuditPage(config *OPNCall, rangeSlug string) string {
	if config == nil {
		return "<div class=\"dashboard\"><h2>BorgAUDIT</h2><div class=\"dash-row\"><span class=\"dash-value dash-muted\">awaiting config</span></div></div>"
	}
	if !config.Git.Enable {
		return "<div class=\"dashboard\"><h2>BorgAUDIT &middot; Git Commit History</h2><div class=\"dash-row\"><span class=\"dash-value dash-muted\">git management disabled (OPN_GIT_ENABLE unset), no commit history to audit</span></div></div>"
	}
	window := _auditRanges[rangeSlug]
	label := auditRangeLabel(rangeSlug)
	since := time.Now().Add(-window)
	commits, err := gatherAuditCommits(config, since)
	if err != nil {
		return "<div class=\"dashboard\"><h2>BorgAUDIT &middot; Git Commit History &middot; " + html.EscapeString(label) + "</h2><div class=\"dash-row\"><span class=\"dash-value dash-err\">" + html.EscapeString(err.Error()) + "</span></div></div>"
	}
	var s strings.Builder
	s.WriteString("<div class=\"dashboard audit-page\"><h2>BorgAUDIT &middot; Git Commit History &middot; ")
	s.WriteString(html.EscapeString(label))
	s.WriteString("</h2>")
	s.WriteString("<p class=\"cfg-intro\">Detailed git commit log for the backup storage repository. Each entry lists the commit hash, author, date, message, file-change stats and the full unified diff with syntax highlighting. Window: since ")
	s.WriteString(html.EscapeString(since.UTC().Format(time.RFC3339)))
	s.WriteString(" (")
	s.WriteString(strconv.Itoa(len(commits)))
	s.WriteString(" commits).</p>")
	if len(commits) == 0 {
		s.WriteString("<div class=\"audit-empty\"><span class=\"dash-muted\">no commits in the selected window</span></div>")
	} else {
		s.WriteString(renderAuditCommits(commits))
	}
	s.WriteString("</div>")
	return s.String()
}

// auditRangeLabel returns the human-readable label for a range slug.
func auditRangeLabel(slug string) string {
	for _, r := range _auditRangeLabels {
		if r.slug == slug {
			return r.label
		}
	}
	return slug
}

// auditCommit is the rendered view of a single commit on the audit page.
type auditCommit struct {
	hash      string // short hash
	author    string
	email     string
	when      time.Time
	message   string
	files     int
	additions int
	deletions int
	diff      string // raw unified diff text (already truncated to cap)
	diffBytes int
	truncated bool
}

// gatherAuditCommits walks the storage repo git log filtered by since and
// returns the matching commits (newest first) with their diff against the
// immediate parent. The walk is bounded by _auditCap so a huge history within
// the window never stalls the page. It does not call os.Chdir; the repo is
// opened directly against config.Path so it is safe to run from the httpd
// goroutine concurrently with the backup workers.
func gatherAuditCommits(config *OPNCall, since time.Time) ([]auditCommit, error) {
	repo, err := git.PlainOpen(config.Path)
	if err != nil {
		if errors.Is(err, git.ErrRepositoryNotExists) {
			return nil, errors.New("repository not initialised")
		}
		return nil, err
	}
	head, err := repo.Head()
	if err != nil {
		// unborn HEAD: repo exists but has no commits yet.
		return nil, nil
	}
	sinceUTC := since.UTC()
	iter, err := repo.Log(&git.LogOptions{
		From:  head.Hash(),
		Since: &sinceUTC,
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	commits := make([]auditCommit, 0, _auditCap)
	for {
		c, err := iter.Next()
		if err != nil {
			break
		}
		if len(commits) >= _auditCap {
			break
		}
		ac, err := buildAuditCommit(c)
		if err != nil {
			// A single unreadable commit should never blank the whole page;
			// surface a minimal stub so the operator sees the gap.
			ac = auditCommit{
				hash:    shortHash(c.Hash.String()),
				author:  safeAuthorName(c.Author),
				email:   c.Author.Email,
				when:    c.Author.When,
				message: "[unreadable commit: " + err.Error() + "]",
				diff:    "",
			}
		}
		commits = append(commits, ac)
	}
	return commits, nil
}

// buildAuditCommit renders a single go-git commit into an auditCommit view:
// it pulls the file-change stats and the unified diff against the commit's
// first parent. The diff is capped at _auditDiffCap bytes; an over-long diff
// is truncated with a visible marker so the operator knows it was clipped.
// A root commit (no parent) yields an empty diff with files/additions/
// deletions derived from the commit tree entry count.
func buildAuditCommit(c *object.Commit) (auditCommit, error) {
	ac := auditCommit{
		hash:    shortHash(c.Hash.String()),
		author:  safeAuthorName(c.Author),
		email:   c.Author.Email,
		when:    c.Author.When,
		message: strings.TrimSpace(c.Message),
	}
	stats, err := c.Stats()
	if err == nil {
		ac.files = len(stats)
		for _, fs := range stats {
			ac.additions += fs.Addition
			ac.deletions += fs.Deletion
		}
	}
	diff, truncated, err := commitDiffText(c)
	if err != nil {
		return ac, err
	}
	ac.diff = diff
	ac.diffBytes = len(diff)
	ac.truncated = truncated
	return ac, nil
}

// commitDiffText returns the unified diff between the commit and its first
// parent. A root commit (no parent) returns an empty diff. The output is
// capped at _auditDiffCap bytes; when it overflows the tail is dropped and a
// truncation marker is appended so the audit page can flag it.
func commitDiffText(c *object.Commit) (string, bool, error) {
	parents := c.ParentHashes
	if len(parents) == 0 {
		// Root commit: diff against the empty tree so the whole initial
		// import is rendered as additions.
		patch, err := c.Patch(nil)
		if err != nil {
			return "", false, err
		}
		diff := patch.String()
		return capDiff(diff)
	}
	parent, perr := c.Parents().Next()
	if perr != nil {
		return "", false, perr
	}
	patch, err := c.Patch(parent)
	if err != nil {
		return "", false, err
	}
	diff := patch.String()
	return capDiff(diff)
}

// capDiff truncates a unified diff to _auditDiffCap bytes and appends a
// visible truncation marker when it overflows.
func capDiff(diff string) (string, bool, error) {
	if len(diff) <= _auditDiffCap {
		return diff, false, nil
	}
	return diff[:_auditDiffCap] + "\n--- DIFF TRUNCATED ---\n", true, nil
}

// shortHash returns the 7-character short form of a git hash.
func shortHash(h string) string {
	if len(h) > 7 {
		return h[:7]
	}
	return h
}

// safeAuthorName returns a non-empty author name, falling back to the email
// local-part when the name is empty (go-git allows empty names).
func safeAuthorName(s object.Signature) string {
	if s.Name != "" {
		return s.Name
	}
	if s.Email != "" {
		return s.Email
	}
	return "(unknown)"
}

// _auditDetailedAnalysisMarker is the header line that separates the TLDR
// headline from the full analysis body in an Ollama-assisted commit message
// (see assembleAnnotatedCommitMessage in ollama.go).
const _auditDetailedAnalysisMarker = "Detailed Analysis:"

// splitAuditMessage splits an Ollama-assisted commit message into the TLDR
// headline and the detailed analysis body. Messages that carry no TLDR
// headline (the default "opnborg auto update" message, the .unf bypass, or a
// plain manual commit) return an empty tldr and the full message as the body,
// so the audit page renders them unchanged.
func splitAuditMessage(msg string) (tldr, detailed string) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return "", ""
	}
	if !strings.HasPrefix(msg, _authorNameTldrPrefix) {
		return "", msg
	}
	head, body, ok := strings.Cut(msg, _auditDetailedAnalysisMarker)
	if !ok {
		return "", msg
	}
	tldr = strings.TrimSpace(head)
	detailed = strings.TrimSpace(body)
	return tldr, detailed
}

// renderAuditCommits emits the per-commit list HTML. Each commit is a
// collapsible <details> card carrying the header (hash, author, date, file
// stats), the commit message, and the syntax-highlighted unified diff.
//
// When the commit message carries an Ollama TLDR headline the summary is
// rendered as a highlighted banner and the detailed analysis is hidden behind
// a collapsible toggle (collapsed by default) so an operator can scan the
// headlines and expand the body only for commits that warrant a closer look.
func renderAuditCommits(commits []auditCommit) string {
	var s strings.Builder
	s.WriteString("<div class=\"audit-list\">")
	for _, c := range commits {
		s.WriteString("<details class=\"audit-commit\" open>")
		s.WriteString("<summary class=\"audit-commit-head\">")
		s.WriteString("<span class=\"audit-hash\">")
		s.WriteString(html.EscapeString(c.hash))
		s.WriteString("</span>")
		s.WriteString("<span class=\"audit-author\">")
		s.WriteString(html.EscapeString(c.author))
		s.WriteString("</span>")
		s.WriteString("<span class=\"audit-date\">")
		s.WriteString(html.EscapeString(c.when.UTC().Format("2006-01-02 15:04:05 Z07:00")))
		s.WriteString("</span>")
		s.WriteString("<span class=\"audit-stats\">")
		s.WriteString(strconv.Itoa(c.files))
		s.WriteString(" files &middot; <span class=\"dash-ok\">+")
		s.WriteString(strconv.Itoa(c.additions))
		s.WriteString("</span> <span class=\"dash-err\">-")
		s.WriteString(strconv.Itoa(c.deletions))
		s.WriteString("</span></span>")
		s.WriteString("</summary>")
		tldr, detailed := splitAuditMessage(c.message)
		if tldr == "" {
			s.WriteString("<pre class=\"audit-message\">")
			s.WriteString(html.EscapeString(c.message))
			s.WriteString("</pre>")
		} else {
			s.WriteString("<div class=\"audit-tldr\">")
			s.WriteString(html.EscapeString(tldr))
			s.WriteString("</div>")
			if detailed != "" {
				s.WriteString("<details class=\"audit-analysis\"><summary class=\"audit-analysis-head\">Detailed Analysis</summary><pre class=\"audit-message audit-analysis-body\">")
				s.WriteString(html.EscapeString(detailed))
				s.WriteString("</pre></details>")
			}
		}
		if c.diff == "" {
			s.WriteString("<div class=\"audit-diff-empty\"><span class=\"dash-muted\">no diff (root commit or binary-only changes)</span></div>")
		} else {
			s.WriteString("<details class=\"audit-diff\"><summary class=\"audit-diff-head\">unified diff")
			if c.truncated {
				s.WriteString(" <span class=\"dash-warn\">[truncated at ")
				s.WriteString(humanBytes(int64(_auditDiffCap)))
				s.WriteString("]</span>")
			}
			s.WriteString("</summary><pre class=\"audit-diff-body\"><code>")
			s.WriteString(highlightDiff(c.diff))
			s.WriteString("</code></pre></details>")
		}
		s.WriteString("</details>")
	}
	s.WriteString("</div>")
	return s.String()
}

// highlightDiff renders a unified diff text as syntax-highlighted HTML. It
// colorises the diff structure (file headers, hunk markers, +/- lines) and
// applies basic XML syntax highlighting to the content of every line so an
// operator can read OPNsense config.xml diffs at a glance. The input is
// expected to be the raw patch text from go-git; the output is fully
// HTML-escaped and safe to embed.
func highlightDiff(text string) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	var b strings.Builder
	b.Grow(len(text))
	for i, line := range lines {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(highlightDiffLine(line))
	}
	return b.String()
}

// highlightDiffLine renders a single diff line with the appropriate diff line
// class and XML syntax tokens highlighted within. It operates on the raw line
// and emits HTML-escaped output so the result is safe to embed.
func highlightDiffLine(line string) string {
	class := diffLineClass(line)
	return "<span class=\"" + class + "\">" + highlightXMLLine(line) + "</span>"
}

// diffLineClass maps a unified-diff line to its CSS class for coloring.
func diffLineClass(line string) string {
	switch {
	case strings.HasPrefix(line, "diff --git"):
		return "diff-file"
	case strings.HasPrefix(line, "index "),
		strings.HasPrefix(line, "similarity "),
		strings.HasPrefix(line, "rename "),
		strings.HasPrefix(line, "copy "),
		strings.HasPrefix(line, "old mode"),
		strings.HasPrefix(line, "new mode"),
		strings.HasPrefix(line, "deleted file"),
		strings.HasPrefix(line, "new file"):
		return "diff-meta"
	case strings.HasPrefix(line, "--- "), strings.HasPrefix(line, "+++ "):
		return "diff-path"
	case strings.HasPrefix(line, "@@"):
		return "diff-hunk"
	case strings.HasPrefix(line, "+"):
		return "diff-add"
	case strings.HasPrefix(line, "-"):
		return "diff-del"
	case strings.HasPrefix(line, "\\"):
		return "diff-meta"
	default:
		return "diff-ctx"
	}
}

// highlightXMLLine scans a raw line for XML markup (comments, processing
// instructions, element tags) and emits HTML-escaped output with the markup
// tokens wrapped in syntax-highlight spans. Text outside any markup is
// HTML-escaped verbatim. A line with no "<" is escaped as-is.
func highlightXMLLine(line string) string {
	if !strings.Contains(line, "<") {
		return html.EscapeString(line)
	}
	var b strings.Builder
	b.Grow(len(line))
	i := 0
	for i < len(line) {
		idx := strings.IndexByte(line[i:], '<')
		if idx < 0 {
			b.WriteString(html.EscapeString(line[i:]))
			break
		}
		b.WriteString(html.EscapeString(line[i : i+idx]))
		i += idx
		rest := line[i:]
		switch {
		case strings.HasPrefix(rest, "<!--"):
			end := strings.Index(rest, "-->")
			if end < 0 {
				b.WriteString("<span class=\"xml-comment\">")
				b.WriteString(html.EscapeString(rest))
				b.WriteString("</span>")
				i = len(line)
			} else {
				end += len("-->")
				b.WriteString("<span class=\"xml-comment\">")
				b.WriteString(html.EscapeString(rest[:end]))
				b.WriteString("</span>")
				i += end
			}
		case strings.HasPrefix(rest, "<?"):
			end := strings.Index(rest, "?>")
			if end < 0 {
				b.WriteString("<span class=\"xml-decl\">")
				b.WriteString(html.EscapeString(rest))
				b.WriteString("</span>")
				i = len(line)
			} else {
				end += len("?>")
				b.WriteString("<span class=\"xml-decl\">")
				b.WriteString(html.EscapeString(rest[:end]))
				b.WriteString("</span>")
				i += end
			}
		default:
			end := strings.IndexByte(rest, '>')
			if end < 0 {
				// no closing > on this line; emit escaped remainder.
				b.WriteString(html.EscapeString(rest))
				i = len(line)
			} else {
				end += len(">")
				b.WriteString(highlightXMLTagRaw(rest[:end]))
				i += end
			}
		}
	}
	return b.String()
}

// highlightXMLTagRaw renders a single raw element tag (from "<" to ">") with
// the tag name and attribute name/value pairs individually colored. Every
// piece is HTML-escaped so the output is safe to embed.
// highlightXMLTagRaw renders a single raw element tag (from "<" to ">") with
// the tag name and attribute name/value pairs individually colored. Every
// piece is HTML-escaped so the output is safe to embed.
// highlightXMLTagRaw renders a single raw element tag (from "<" to ">") with
// the tag name and attribute name/value pairs individually colored. Every
// piece is HTML-escaped so the output is safe to embed.
func highlightXMLTagRaw(tag string) string {
	if !strings.HasPrefix(tag, "<") || !strings.HasSuffix(tag, ">") {
		return html.EscapeString(tag)
	}
	body := tag[1 : len(tag)-1]
	selfClose := strings.HasSuffix(body, "/")
	if selfClose {
		body = body[:len(body)-1]
	}
	closing := strings.HasPrefix(body, "/")
	if closing {
		body = body[1:]
	}
	var b strings.Builder
	b.WriteString("<span class=\"xml-tag\">")
	b.WriteString(html.EscapeString("<"))
	if closing {
		b.WriteString("/")
	}
	sp := strings.IndexAny(body, " \t")
	if sp < 0 {
		b.WriteString("<span class=\"xml-name\">")
		b.WriteString(html.EscapeString(body))
		b.WriteString("</span>")
	} else {
		name := body[:sp]
		attrs := strings.TrimSpace(body[sp:])
		b.WriteString("<span class=\"xml-name\">")
		b.WriteString(html.EscapeString(name))
		b.WriteString("</span>")
		b.WriteString(highlightXMLAttrsRaw(attrs))
	}
	if selfClose {
		b.WriteString("/")
	}
	b.WriteString(html.EscapeString(">"))
	b.WriteString("</span>")
	return b.String()
}

// _auditAttrRe splits an attribute string into name="value" pairs. It is
// applied to the raw (un-escaped) attribute body.
var _auditAttrRe = regexp.MustCompile(`([\w:.\-]+)\s*=\s*("[^"]*")`)

// highlightXMLAttrsRaw colorises the attribute list of a tag: attribute names
// in one color, double-quoted values in another. Text outside an attribute
// pair (whitespace, stray tokens) is HTML-escaped and emitted verbatim.
func highlightXMLAttrsRaw(attrs string) string {
	if attrs == "" {
		return ""
	}
	var b strings.Builder
	last := 0
	for _, m := range _auditAttrRe.FindAllStringSubmatchIndex(attrs, -1) {
		b.WriteString(html.EscapeString(attrs[last:m[0]]))
		b.WriteString("<span class=\"xml-attr-name\">")
		b.WriteString(html.EscapeString(attrs[m[2]:m[3]]))
		b.WriteString("</span>=")
		b.WriteString("<span class=\"xml-attr-val\">")
		b.WriteString(html.EscapeString(attrs[m[4]:m[5]]))
		b.WriteString("</span>")
		last = m[1]
	}
	b.WriteString(html.EscapeString(attrs[last:]))
	return b.String()
}

// highlightXMLTokens is a backward-compat wrapper that HTML-escapes its input
// before delegating to highlightXMLLine. Callers that already hold raw text
// should call highlightXMLLine directly.
func highlightXMLTokens(raw string) string {
	return highlightXMLLine(raw)
}
