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
		seen := "<td><b>Last Seen: " + ts.Format(time.RFC3339) + "</b></td>"
		ver := getFirmwareVersion(config, server)
		borgSC := "<a href=\"https://" + server + _srvc + "\" " + _nwin + "><button><img src=\"favicon.ico\" width=\"12\" height=\"12\"></button></a>"
		linkUI := "<a href=\"https://" + server + _dash + "\" " + _nwin + "><button><b>[" + server + "]</b></button></a> " + borgSC
		linkVS := "<a href=\"https://" + server + _fwup + "\" " + _nwin + "><button><b>[" + ver + "]</b></button></a>"
		linkCurrent := "<a href=\"./files/" + server + "/current.xml\"" + _nwin + "><button><b>[current.xml]</b></button></a>"
		linkArchive := "<a href=\"./files/" + server + "/" + archive + "\" " + _nwin + "><button><b>[archive]</b></button></a>"
		links := " <td>" + linkCurrent + " " + linkArchive + "</td>"
		if tag != "" {
			tag = "<td><b>" + tag + "</b></tb>"
		}
		status := state + " </td><td>" + linkUI + " " + linkVS + links + seen + tag
		hiveMutex.Lock()
		hive[id] = status
		hiveMutex.Unlock()
		return
	}
	hiveMutex.Lock()
	defer hiveMutex.Unlock()
	status := hive[id]
	status = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(status, _ok, ""), _na, ""), _fail, ""), _degraded, "")
	status = _fail + status
	hive[id] = status
}

// setUnifiStatus
func setUnifiStatus(config *OPNCall, server, tag, notice string, ts time.Time, responsive, backup bool) {
	// lock
	unifiMutex.Lock()
	defer unifiMutex.Unlock()

	// clean status
	unifiStatus = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(unifiStatus, _unifi, ""), _na, ""), _fail, ""), _degraded, "")

	// setup
	year, month, _ := ts.Date()
	archive := filepath.Join(_archive, strconv.Itoa(year), padMonth(strconv.Itoa(int(month))))

	if responsive {
		state := _unifi
		seen := "<td><b>Last Seen: " + ts.Format(time.RFC3339) + "</b></td>"
		linkUI := "<a href=\"" + config.Unifi.WebUI.String() + "\" " + _nwin + "><button><b>[" + server + "]</b></button></a> "
		linkCurrent := "<a href=\"./files/" + server + "/current.unf\"" + _nwin + "><button><b>[current.unf]</b></button></a>"
		linkArchive := "<a href=\"./files/" + server + "/" + archive + "\" " + _nwin + "><button><b>[archive]</b></button></a>"
		links := " <td>" + linkCurrent + " " + linkArchive + "</td>"
		if !backup {
			state = _degraded
		}
		export := ""
		if config.Unifi.Export.Enable {
			ext := config.Unifi.Export.Format
			exportCurrent := "<a href=\"./files/" + _uniEx + "/current." + ext + "\"" + _nwin + "><button><b>[current." + ext + "]</b></button></a>"
			exportArchive := "<a href=\"./files/" + _uniEx + "/" + archive + "\" " + _nwin + "><button><b>[archive]</b></button></a>"
			export = " <td><b>Export: </b>" + exportCurrent + " " + exportArchive + "</td>"
		}
		if tag != "" {
			tag = "<td><b>" + tag + "</b>"
		}
		unifiStatus = state + " </td><td>" + linkUI + " " + links + export + seen + tag
		return
	}
	unifiStatus = _fail + unifiStatus
}
