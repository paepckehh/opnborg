package opnborg

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

// ReadConfig reads OPNBorgs configuration via env
func ReadConfig() (*OPNCall, error) {

	if _, ok := os.LookupEnv("OPN_TARGETS"); !ok {
		return nil, errors.New(fmt.Sprintf("Add at least one target server to env var 'OPN_TARGETS' (multi valued, comma seperated)"))
	}

	if _, ok := os.LookupEnv("OPN_APIKEY"); !ok {
		return nil, errors.New(fmt.Sprintf("Set env var 'OPN_APIKEY' to your opnsense api key"))
	}

	if _, ok := os.LookupEnv("OPN_APISECRET"); !ok {
		return nil, errors.New(fmt.Sprintf("Set env var 'OPN_APISECRET' to your opnsense api key secret"))
	}
	return &OPNCall{
		Targets:   os.Getenv("OPN_TARGETS"),
		Key:       os.Getenv("OPN_APIKEY"),
		Secret:    os.Getenv("OPN_APISECRET"),
		Path:      os.Getenv("OPN_PATH"),
		TLSKeyPin: os.Getenv("OPN_TLSKEYPIN"),
		Daemon:    os.Getenv("OPN_DAEMON") == "0",
		Git:       os.Getenv("OPN_NOGIT") == "0",
		SSL:       os.Getenv("OPN_NOSSL") == "1",
	}, nil
}

// Backup performs a Backup operation
func Backup(config *OPNCall) error {

	// setup
	var wg sync.WaitGroup
	if config.AppName == "" {
		config.AppName = "[OPNBORG-API]"
	}

	// spinup Log/Display Engine
	display.Add(1)
	go startLog(config)

	// spinup individual worker for every server
	displayChan <- []byte("[STARTING][BACKUP]")
	for _, server := range strings.Split(config.Targets, ",") {
		wg.Add(1)
		go backupSrv(server, config, &wg)
	}

	// wait till all worker done
	wg.Wait()
	displayChan <- []byte("[FINISH][BACKUP][ALL]")
	close(displayChan)
	display.Wait()
	return nil
}
