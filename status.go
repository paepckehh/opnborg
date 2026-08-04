package opnborg

import (
	"html"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	_dash = "/ui/core/dashboard"
	_fwup = "/ui/core/firmware#status"
	_plug = "/ui/core/firmware#plugins"
	_srvc = "/ui/core/service"
	_nwin = "target=\"_blank\""
)

// setOPNStatus sets the hive member server status
func setOPNStatus(config *OPNCall, server, tag, notice string, id int, ts time.Time, degraded, ok bool) {
	year, month, _ := ts.Date()
	archive := filepath.Join(_archive, strconv.Itoa(year), padMonth(strconv.Itoa(int(month))))
	if ok {
		state := _ok
		if degraded {
			state = _degraded
			if notice != "" {
				state = strings.ReplaceAll(state, "DEGRADED", html.EscapeString(notice))
			}
		}
		seen := "<div class=\"meta-box meta-last-seen\"><span class=\"meta-label\">Last Seen</span><span class=\"meta-value\">" + ts.Format(time.RFC3339) + "</span></div>"
		ver := getFirmwareVersion(config, server)
		borgSC := "<a href=\"https://" + server + _srvc + "\" " + _nwin + "><button><img src=\"favicon.ico\" width=\"12\" height=\"12\"></button></a>"
		linkUI := "<a href=\"https://" + server + _dash + "\" " + _nwin + "><button>[" + server + "]</button></a>" + borgSC
		linkVS := "<a href=\"https://" + server + _fwup + "\" " + _nwin + "><button>[" + ver + "]</button></a>"
		linkCurrent := "<a href=\"./files/" + server + "/current.xml\"" + _nwin + "><button>[current.xml]</button></a>"
		linkArchive := "<a href=\"./files/" + server + "/" + archive + "\" " + _nwin + "><button>[archive]</button></a>"
		links := "<span class=\"member-links member-links-backup\">" + linkCurrent + linkArchive + "</span>"
		tagBox := ""
		if tag != "" {
			tagBox = "<div class=\"meta-box meta-tag\"><span class=\"meta-label\">Tag</span><span class=\"meta-value\">" + html.EscapeString(tag) + "</span></div>"
		}
		status := "<div class=\"member-status\">" + state + "</div><div class=\"member-main\"><span class=\"member-links member-links-ui\">" + linkUI + linkVS + "</span>" + links + "</div>" + seen + tagBox
		hiveMutex.Lock()
		hive[id] = status
		hiveMutex.Unlock()
		return
	}
	hiveMutex.Lock()
	defer hiveMutex.Unlock()
	status := hive[id]
	status = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(status, _ok, ""), _na, ""), _fail, ""), _degraded, "")
	status = strings.Replace(status, "<div class=\"member-status\"></div>", "", 1)
	status = "<div class=\"member-status\">" + _fail + "</div>" + status
	hive[id] = status
}

// setUnifiStatus
func setUnifiStatus(config *OPNCall, server, tag, notice string, ts time.Time, responsive, backup bool) {
	// lock
	unifiMutex.Lock()
	defer unifiMutex.Unlock()

	// setup
	year, month, _ := ts.Date()
	archive := filepath.Join(_archive, strconv.Itoa(year), padMonth(strconv.Itoa(int(month))))

	if responsive {
		state := _unifi
		seen := "<div class=\"meta-box meta-last-seen\"><span class=\"meta-label\">Last Seen</span><span class=\"meta-value\">" + ts.Format(time.RFC3339) + "</span></div>"
		linkUI := "<a href=\"" + config.Unifi.WebUI.String() + "\" " + _nwin + "><button>[" + server + "]</button></a>"
		linkCurrent := "<a href=\"./files/" + server + "/current.unf\"" + _nwin + "><button>[current.unf]</button></a>"
		linkArchive := "<a href=\"./files/" + server + "/" + archive + "\" " + _nwin + "><button>[archive]</button></a>"
		links := "<span class=\"member-links member-links-backup\">" + linkCurrent + linkArchive + "</span>"
		if !backup {
			state = _degraded
			if notice != "" {
				state = strings.ReplaceAll(state, "DEGRADED", html.EscapeString(notice))
			}
		}
		export := ""
		if config.Unifi.Export.Enable {
			ext := config.Unifi.Export.Format
			exportCurrent := "<a href=\"./files/" + _uniEx + "/current." + ext + "\"" + _nwin + "><button>[current." + ext + "]</button></a>"
			exportArchive := "<a href=\"./files/" + _uniEx + "/" + archive + "\" " + _nwin + "><button>[archive]</button></a>"
			export = "<span class=\"member-links member-links-export\">" + exportCurrent + exportArchive + "</span>"
		}
		tagBox := ""
		if tag != "" {
			tagBox = "<div class=\"meta-box meta-tag\"><span class=\"meta-label\">Tag</span><span class=\"meta-value\">" + html.EscapeString(tag) + "</span></div>"
		}
		unifiStatus = "<div class=\"member-status\">" + state + "</div><div class=\"member-main\"><span class=\"member-links member-links-ui\">" + linkUI + "</span>" + links + export + "</div>" + seen + tagBox
		return
	}
	// clean status: strip any state svg, drop the now-empty status wrapper, and
	// re-render with the failure indicator while preserving the meta boxes.
	unifiStatus = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(unifiStatus, _unifi, ""), _na, ""), _fail, ""), _degraded, "")
	unifiStatus = strings.Replace(unifiStatus, "<div class=\"member-status\"></div>", "", 1)
	unifiStatus = "<div class=\"member-status\">" + _fail + "</div>" + unifiStatus
}

// setUnifiWatchStatus renders the Unifi autoBackup folder-watch sync tile.
// It mirrors the Unifi backup tile (green when the watcher is responsive and
// the last sync succeeded, degraded otherwise) and surfaces the last sync
// date from the mtime of the autobackup_meta.json marker file plus archive
// and current backup buttons, all under the Unifi branding/logo.
func setUnifiWatchStatus(config *OPNCall, responsive, syncOK bool) {
	unifiWatchMutex.Lock()
	defer unifiWatchMutex.Unlock()

	year, month, _ := config.Unifi.Watch.LastTS.Date()
	archive := filepath.Join(_archive, strconv.Itoa(year), padMonth(strconv.Itoa(int(month))))
	lastSeen := "n/a"
	if !config.Unifi.Watch.LastTS.IsZero() {
		lastSeen = config.Unifi.Watch.LastTS.Format(time.RFC3339)
	}
	server := "unifi-autobackup"
	uiLink := ""
	if config.Unifi.WebUI != nil {
		server = config.Unifi.WebUI.Hostname()
		if server == "" {
			server = "unifi-autobackup"
		}
		uiLink = config.Unifi.WebUI.String()
	}

	if !responsive {
		unifiWatchStatus = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(unifiWatchStatus, _unifi, ""), _ok, ""), _na, ""), _fail, "")
		unifiWatchStatus = strings.Replace(unifiWatchStatus, "<div class=\"member-status\"></div>", "", 1)
		unifiWatchStatus = "<div class=\"member-status\">" + _fail + "</div>" + unifiWatchStatus
		if unifiWatchStatus == "<div class=\"member-status\">"+_fail+"</div>" {
			unifiWatchStatus = "<div class=\"member-status\">" + _fail + "</div><div class=\"member-main\"><span class=\"member-meta\">Unifi autoBackup Watch: unreachable Last Sync: " + lastSeen + "</span></div>"
		}
		return
	}

	state := _ok
	if !syncOK {
		state = _degraded
	}
	seen := "<div class=\"meta-box meta-last-seen\"><span class=\"meta-label\">Last Sync</span><span class=\"meta-value\">" + lastSeen + "</span></div>"
	linkUI := "<a href=\"" + uiLink + "\" " + _nwin + "><button>[" + server + "]</button></a>"
	linkCurrent := "<a href=\"./files/" + _uniWatch + "/current.unf\"" + _nwin + "><button>[current.unf]</button></a>"
	linkArchive := "<a href=\"./files/" + _uniWatch + "/" + archive + "\" " + _nwin + "><button>[archive]</button></a>"
	links := "<span class=\"member-links member-links-backup\">" + linkCurrent + linkArchive + "</span>"
	tagBox := ""
	if config.Unifi.Tag != "" {
		tagBox = "<div class=\"meta-box meta-tag\"><span class=\"meta-label\">Tag</span><span class=\"meta-value\">" + html.EscapeString(config.Unifi.Tag) + "</span></div>"
	}
	unifiWatchStatus = "<div class=\"member-status\">" + state + "</div><div class=\"member-main\"><span class=\"member-links member-links-ui\">" + linkUI + "</span>" + links + "</div>" + seen + tagBox
}
