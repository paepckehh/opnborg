# OVERVIEW 
[![Go Reference](https://pkg.go.dev/badge/paepcke.de/opnborg.svg)](https://pkg.go.dev/paepcke.de/opnborg) 
[![Go Report Card](https://goreportcard.com/badge/paepcke.de/opnborg)](https://goreportcard.com/report/paepcke.de/opnborg) 

[paepcke.de/opnborg](https://paepcke.de/opnborg/)

# OPNBORG 

- secure backup & orchestration for [opnsense.org](https://opnsense.org/) appliances
  
# EXAMPLE 
```
OPN_TARGETS="opn001.lan,opn002.lan,opn003.lan" OPN_APIKEY="..." OPN_APISECRET="..." OPN_NOSSL="true" go run paepcke.de/opnborg/cmd/opnborg@latest
```

# SUPPORTED OPTIONS 

```
# REQUIRED: 
- OPN_TARGETS   - list of OPNSense Target Server to Backup
- OPN_APIKEY    - OPNsense Backup User APIKEY
- OPN_APISECRET - OPNsense Backup User APISECRET

# OPTIONAL:
- OPN_TLSKEYPIN - OPNsense TLS MitM proof Certificate Keypin [string]
- OPN_DAEMON    - run app in daemon mode, never quit, fetch once every hour [bool: defaults to 'false']
- OPN_NOGIT     - do not create & update local git version repo [bool: defaults to 'false']
- OPN_NOSSL     - do not verify SSL Certificates [bool: defaults to *'true'*, is pointless, use OPN_TLSKEYPIN!]

```
# OPTIONS FAQ
- How to create a secure OPENSense Backup OPN_APIKEY & OPN_APISECRET? 
    
    - Create a User 'backup' 
        - OPNSense WebUI -> System -> Access -> User -> Add 
        - Skip passwords, tick scrambled random password & tick 'Save and go back' 
    
    - Go back to user 'backup' via -> Edit (new option sections magically appear)
        - Effective Privileges -> Edit 
            - Diagnostics: Configuration History (tick allowed box & 'Save' button)
        - API Keys -> Add (Create API Key): The API Key & Secret will download to your browser download folder as file

- How to lock down the TLS Session MitM proof via 'OPN_TLSKEYPIN'? 
    
    - enable https for your OPNSense Admin interface (even simple self-signed certificates will do the trick)
    
    - go run paepcke.de/tlsinfo/cmd/tlsinfo@latest <your-opn-server-name>
        - Pick First Line (copy only the base64 encoded string, without brackets): 
            Example:    X509 Cert KeyPin [base64] : [FezOCC3qZFzBmD5xRKtDoLgK445Kr0DeJBj2TWVvR9M=]
                        OPN_TLSKEYPIN='FezOCC3qZFzBmD5xRKtDoLgK445Kr0DeJBj2TWVvR9M='

- Enviroment Variables bools must set exactly case senitive to 'true' to enable, anything else will default to false.
- Clear text HTTP protocol is not supported, switch on HTTPS for your admin interface (self-signed certificates will do)

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

