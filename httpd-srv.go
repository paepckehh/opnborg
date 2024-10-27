package opnborg

import (
	"fmt"
	"net/http"
	"os"
)

// httpd spinup the http internal web server
func startWeb(config *OPNCall) {

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
	mux.Handle("/gitlog/", addSecurityHeader(getGitHandler()))
	mux.Handle("/files/", addSecurityHeader(http.StripPrefix("/files/", http.FileServer(http.Dir(config.Path)))))
	mux.Handle("/favicon.ico", getFavIconHandler())

	// httpsrv
	httpsrv := &http.Server{
		Handler: mux,
	}

	// info
	displayChan <- []byte("[HTTPD-SRV][SPIN-UP-SERVER] " + config.Httpd.Server)

	// serve requestes, print err after httpd crash
	fmt.Println(httpsrv.Serve(listener))
}
