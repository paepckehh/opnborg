package opnborg

import (
	"fmt"
	"net/http"
	"os"

	githttp "github.com/AaronO/go-git-http"
)

// httpd spinup the http internal web server
func startWeb(c *OPNCall) {

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
	mux.Handle("/files/", addSecurityHeader(http.StripPrefix("/files/", http.FileServer(http.Dir(c.Path)))))
	mux.Handle("/force", getForceHandler())
	mux.Handle("/favicon.ico", getFavIconHandler())

	// spin up internal git repo https server
	state := "[DISABLED]"
	if c.GitSrv.Enable {
		mux.Handle("/git", githttp.New(c.Path))
		state = "[ENABLED]"
	}
	displayChan <- []byte("[GITSRV-HTTP]" + state)

	// httpsrv
	httpsrv := &http.Server{
		Handler: mux,
	}

	// info
	displayChan <- []byte("[HTTPD-SRV][SPIN-UP-SERVER] " + c.Httpd.Server)

	// serve requestes, print err after httpd crash
	fmt.Println(httpsrv.Serve(listener))
}
