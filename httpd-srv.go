package opnborg

import (
	"fmt"
	"net/http"
	"os"
)

// httpd spinup the http internal web server
func startWeb(config *OPNCall) {

	// terminate if not in daemon mode
	if !config.Daemon {
		return
	}
	// create store structure
	if err := os.MkdirAll(config.Path, 0770); err != nil {
		fmt.Println(err)
		return
	}

	// change thread into store-root
	if err := os.Chdir(config.Path); err != nil {
		fmt.Println(err)
		return
	}

	// get listener, bind ports
	listener, err := getHTTPTLS(config)
	if err != nil {
		fmt.Println(err)
		return
	}

	// setup mux
	mux := http.NewServeMux()

	// handler
	mux.Handle("/", addSecurityHeader(getIndexHandler()))
	mux.Handle("/files/", addSecurityHeader(http.StripPrefix("/files/", http.FileServer(http.Dir(config.Path)))))
	// mux.Handle("/icon.svg", getFavIconHandler())

	// httpsrv
	httpsrv := &http.Server{
		Handler: mux,
	}

	// serve requestes, print err after httpd crash
	fmt.Println(httpsrv.Serve(listener))
}
