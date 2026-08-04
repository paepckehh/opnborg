package opnborg

import (
	"fmt"
	"net/http"
	"os"
)

// httpd spinup the http internal web server
func startWeb(c *OPNCall) {

	// capture the live config handle for the WebUI render path (the dashboard
	// gathers backup folder + git state on every render and needs the storage
	// path / git settings).
	_cfg = c

	// create store structure
	if err := os.MkdirAll(c.Path, 0770); err != nil {
		fmt.Println(err)
		return
	}

	// change thread into store-root
	if err := os.Chdir(c.Path); err != nil {
		fmt.Println(err)
		return
	}

	// get listener, bind ports
	listener, err := getHTTPTLS(c)
	if err != nil {
		fmt.Println(err)
		return
	}

	// setup mux
	mux := http.NewServeMux()

	// handler
	mux.Handle("/", addSecurityHeader(getIndexHandler()))
	mux.Handle("/config", addSecurityHeader(getConfigDashboardHandler()))
	mux.Handle("/files/", addSecurityHeader(http.StripPrefix("/files/", http.FileServer(http.Dir(c.Path)))))
	mux.Handle("/force", getForceHandler())
	mux.Handle("/favicon.ico", getFavIconHandler())

	// httpsrv
	httpsrv := &http.Server{
		Handler: mux,
	}

	// info
	displayChan <- []byte("[HTTPD-SRV][SPIN-UP-SERVER] " + c.Httpd.Server)

	// serve requestes, print err after httpd crash
	fmt.Println(httpsrv.Serve(listener))
}
