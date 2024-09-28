# OVERVIEW 
[![Go Reference](https://pkg.go.dev/badge/paepcke.de/opnborg.svg)](https://pkg.go.dev/paepcke.de/opnborg) 
[![Go Report Card](https://goreportcard.com/badge/paepcke.de/opnborg)](https://goreportcard.com/report/paepcke.de/opnborg) 

[paepcke.de/opnborg](https://paepcke.de/opnborg/)

# OPNBORG 

- secure backup & orchestration for [opnsense.org](https://opnsense.org/) appliances
  
# EXAMPLE 
```
OPN_TARGETS="opn01.lan,opn02.lan" OPN_APIKEY="..." OPN_APISECRET="..." go run paepcke.de/opnborg/cmd/opnborg@latest
```

# EXAMPLE ENV CONFIG
```
please see:
- example.sh 
- example-env-config.sh
```

# SUPPORTED OPTIONS 

```
# REQUIRED: 
- OPN_TARGETS     - list of OPNSense Target Server to Backup [string, hostnames, comma separated]
- OPN_APIKEY      - OPNsense Backup User APIKEY [string, base64 encoded]
- OPN_APISECRET   - OPNsense Backup User APISECRET [string, base64 encoded]

# OPTIONAL:
- OPN_PATH        - specify a target path (absolut or releative) to store backups [string: defaults to '.']
- OPN_TLSKEYPIN   - OPNsense TLS MitM proof Certificate Keypin [string]
- OPN_SLEEP       - daemon mode poll interval [string, defaults to 3600 seconds, minimum 5 seconds]
- OPN_EMAIL       - specify email address contact for local git commits [string: defaults to git@opnborg]
- OPN_NODAEMON    - do not run app in daemon mode, quit after one loop [bool: defaults to 'false']
- OPN_NOGIT       - do not create & update local git version repo [bool: defaults to 'false']
- OPN_DEBUG       - verbose debug log mode [bool: defaults to 'false']

# OPN Orchestrator 
- OPN_MASTER      - define a master server, opnborg will replicate all config changes on the master to all the hive [string, hostname]
- OPN_SYNC_PKG    - enable to unlock opnsense hive package (system/plugin) syncronisation across all targets [bool, defaults to false]

# OPN Operations 
- OPN_RSYSLOG_ENABLE - spin up internal RFC5424 rsyslog server, configure all hive member to log (bool, default: false)
- OPN_RSYSLOG_SERVER - [required] define syslog srv listen ip & port [example: 192.168.0.1:5140] (Do not use 0.0.0.0, its srv & target ip conf!)

# OPN WebConsole 
- OPN_HTTPD_ENABLE      - spin up internal httpd server (bool, default: false)
- OPN_HTTPD_SERVER      - HTTPD Listen Address  [string, default: 127.0.0.1:80] 
- OPN_HTTPD_CACERT      - HTTPD Server CA X.509 Certificate (string: <server.pem>), defaults to <empty>, empty disables https)
- OPN_HTTPD_CACKEY      - HTTPD Server CA Key  (string: <server.key>), defaults to <empty>, empty disables https)
- OPN_HTTPD_CACLIENT    - HTTPD Server CA ClientCA Certificate (string: <clientCA.pem>), defaults to <empty>, if set, enforces mTLS)

```
# OPTIONS FAQ

```
- How to create a secure OPENSense Backup API Key? (OPN_APIKEY & OPN_APISECRET)
    
    - Create a User 'backup' 
        - OPNSense WebUI -> System -> Access -> User -> Add 
        - Skip passwords, tick scrambled random password & tick 'Save and go back' 
    
    - Go back to user 'backup' via -> Edit (new option sections magically appear)
        - Effective Privileges -> Edit 
            - Diagnostics: Configuration History -> tick allowed box [for system backups]
            - System: Firmware -> tick allowed box [only needed for automatic plugin/pkg management]
            - click 'Save' button to activate
        - API Keys -> Add (Create API Key): The API Key & Secret will download to your browser download folder as file

- How to lock down the TLS Session MitM proof via 'OPN_TLSKEYPIN'? 
    
    - enable https for your OPNSense Admin interface (even simple self-signed certificates will do the trick)
    
    - go run paepcke.de/tlsinfo/cmd/tlsinfo@latest <your-opn-server-name>
        - Pick First Line (copy only the base64 encoded string, without brackets): 
            Example:    X509 Cert KeyPin [base64] : [FezOCC3qZFzBmD5xRKtDoLgK445Kr0DeJBj2TWVvR9M=]
                        OPN_TLSKEYPIN='FezOCC3qZFzBmD5xRKtDoLgK445Kr0DeJBj2TWVvR9M='

- The internal webserver is listening to on [any] interface port 6464 -> 0.0.0.0:6464 (http://localhost:6464) (daemon mode only) 
- Enviroment Variables bools will always be true if defined (the value you set is not relevant)
- Clear text HTTP protocol is not supported, switch on HTTPS for your admin interface (self-signed certificates will do)
- ATT: HTTPS chain verification via system os trust store(s) is disabled by default: use OPN_TLSKEYPIN (!!!)
```

# HOW TO INSTALL

```
go install paepcke.de/opnborg/cmd/opnborg@latest
```

### DOWNLOAD (prebuild)

[github.com/paepckehh/opnborg/releases](https://github.com/paepckehh/opnborg/releases)


# STATUS

 - -=# WORK IN PROGRESS - BUT USABLE #=- 

# TIMELINE 

 - 2024
    - September -> Internal Use Only (codesamples)
    - October   -> Kickoff Offical Testphase 
    - November  -> Offical Pilot Phase
    - December  -> Release Candidate
 - 2025
    - January - > First Public Release & Native [NixOS](https://github.com/nixos) support

# CONTRIBUTION

Yes, Please! PRs Welcome! 

# SPONSORS 



