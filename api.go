package opnborg

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

func ReadConfig() (*OPNCall, error) {

	fmt.Println(_app + "[STARTING][READ-CONFIG-FROM-ENV]")
	if _, ok := os.LookupEnv("OPN_TARGETS"); !ok {
		return nil, errors.New(fmt.Sprintf("[ERROR] Add at least one target server to env var 'OPN_TARGETS' (multi valued, comma seperated)"))
	}

	if _, ok := os.LookupEnv("OPN_APIKEY"); !ok {
		return nil, errors.New(fmt.Sprintf("[ERROR] Set env var 'OPN_APIKEY' to your opnsense api key"))
	}

	if _, ok := os.LookupEnv("OPN_APISECRET"); !ok {
		return nil, errors.New(fmt.Sprintf("[ERROR] Set env var 'OPN_APISECRET' to your opnsense api key secret"))
	}
	fmt.Println(_app + "[SUCCESS][READ-CONFIG-FROM-ENV]")
	return &OPNCall{
		Targets:     os.Getenv("OPN_TARGETS"),
		Key:         os.Getenv("OPN_APIKEY"),
		Secret:      os.Getenv("OPN_APISECRET"),
		NoSSLVerify: os.Getenv("OPN_NOSSLVERIFY") == "1",
	}, nil
}

func Backup(conf *OPNCall) error {

	fmt.Println(_app + "[STARTING][BACKUP]")
	for _, server := range strings.Split(conf.Targets, ",") {
		fmt.Println(_app+"[BACKUP][START][PROCESSING-SERVER]: https://", server, "/api")
	}
	fmt.Println(_app + "[SUCCESS][BACKUP]")
	return nil
}
