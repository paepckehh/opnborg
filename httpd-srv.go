package opnborg

import "net/http"

// httpd spinup the http internal web server
func httpd() error {

	// get listener, bind ports
	listener, err := getHTTPTLS()
	if err != nil {
		return err
	}

	// setup mux
	mux := http.NewServeMux()

	// handler
	mux.Handle("/", getIndexHandler())
	mux.Handle("/files/", getFilesHandler())
	mux.Handle("/icon.svg", getFavIconHandler())

	// httpsrv
	httpsrv := &http.Server{
		Handler: mux,
	}

	// serve requestes, return err after crash
	return httpsrv.Serve(listener)
}
