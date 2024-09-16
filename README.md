# OVERVIEW 
[![Go Reference](https://pkg.go.dev/badge/paepcke.de/opnborg.svg)](https://pkg.go.dev/paepcke.de/opnborg) 
[![Go Report Card](https://goreportcard.com/badge/paepcke.de/opnborg)](https://goreportcard.com/report/paepcke.de/opnborg) 

[paepcke.de/opnborg](https://paepcke.de/opnborg/)

# OPNBORG 

- secure backup & orchestration ofn[opnsense.org](https://opnsense.org/) appliances (cluster)
  
# EXAMPLE
```
OPN_TARGETS="opn001.lan,opn002.lan,opn003.lan" OPN_APIKEY="..." OPN_APISECRET="..." OPN_KEYPIN="..." go run paepcke.de/opnborg/cmd/opnborg@latest
```

# SUPPORTED OPTIONS 

```
REQUIRED: 
- OPN_TARGETS   - list of OPNsense Target Server to Backup
- OPN_APIKEY    - OPNsense Backup User APIKEY
- OPN_APISECRET - OPNsense Backup User APISECRET

OPTIONAL:
- OPN_TLSKEYPIN - OPNsense TLS Certificate Keypin 
- OPN_NOSSL     - do not verify SSL Certificate

```
# HOW TO INSTALL

```
go install paepcke.de/opnborg/cmd/opnborg@latest
```

### DOWNLOAD (prebuild)

[github.com/paepckehh/opnborg/releases](https://github.com/paepckehh/opnborg/releases)


# STATUS

 - -=# WORK IN PROGRESS - DO NOT USE THIS REPO - MAY IMPLODE ANYTIME #=- 
 - -=# WORK IN PROGRESS - DO NOT USE THIS REPO - MAY IMPLODE ANYTIME #=- 
 - -=# WORK IN PROGRESS - DO NOT USE THIS REPO - MAY IMPLODE ANYTIME #=- 

# TIMELINE 

 - 2024
    - September -> Internal Use Only (codesamples)
    - October   -> Offical Testphase 
    - November  -> Offical Pilot Phase
    - December  -> Release Candidate
 - 2025
    - January - > First Public Release planned 

# CONTRIBUTION

Yes, Please! PRs Welcome! 

# SPONSORS 

Not Yet!
