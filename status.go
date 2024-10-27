package opnborg

import (
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

// setOPNStatus
func setOPNStatus(config *OPNCall, server string, id int, ts time.Time, degraded, ok bool) {
	year, month, _ := ts.Date()
	archive := filepath.Join(_archive, strconv.Itoa(year), padMonth(strconv.Itoa(int(month))))
	if ok {
		state := _ok
		if degraded {
			state = _degraded
		}
		seen := ts.Format(time.RFC3339)
		ver := getFirmwareVersion(config, server)
		borgSC := "<a href=\"https://" + server + _srvc + "\" " + _nwin + "><button><img src=\"favicon.ico\" width=\"12\" height=\"12\"></button></a>"
		linkUI := "<a href=\"https://" + server + _dash + "\" " + _nwin + "><button><b>[" + server + "]</b></button></a> " + borgSC
		linkVS := "<a href=\"https://" + server + _fwup + "\" " + _nwin + "><button><b>[" + ver + "]</b></button></a>"
		linkCurrent := "<a href=\"./files/" + server + "/current.xml\"" + _nwin + "><button><b>[current.xml]</b></button></a>"
		linkArchive := "<a href=\"./files/" + server + "/" + archive + "\" " + _nwin + "><button><b>[archive]</b></button></a>"
		links := linkCurrent + " " + linkArchive
		status := state + _b + linkUI + _b + linkVS + " <button><b>Last Seen:" + seen + "</b></button> " + links + "<br>"
		hiveMutex.Lock()
		hive[id] = status
		hiveMutex.Unlock()
		return
	}
	hiveMutex.Lock()
	status := hive[id]
	status = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(status, _ok, ""), _na, ""), _fail, ""), _degraded, "")
	status = _fail + status
	hive[id] = status
	hiveMutex.Unlock()
}
