
---

### *** OPNBORG - RESISTANCE IS FUTILE. YOUR OPNSENSE WILL BE ASSIMILATED. ***

---

# OVERVIEW 
[![Go Reference](https://pkg.go.dev/badge/paepcke.de/opnborg.svg)](https://pkg.go.dev/paepcke.de/opnborg) 
[![Go Report Card](https://goreportcard.com/badge/paepcke.de/opnborg)](https://goreportcard.com/report/paepcke.de/opnborg) 
[![Go Build](https://github.com/paepckehh/opnborg/actions/workflows/golang.yml/badge.svg)](https://github.com/paepckehh/opnborg/actions/workflows/golang.yml)
[![License](https://img.shields.io/github/license/paepckehh/opnborg)](https://github.com/paepckehh/opnborg/blob/master/LICENSE)
[![SemVer](https://img.shields.io/github/v/release/paepckehh/opnborg)](https://github.com/paepckehh/opnborg/releases/latest)
<br>[![built with nix](https://builtwithnix.org/badge.svg)](https://search.nixos.org/packages?channel=unstable&from=0&size=50&sort=relevance&type=packages&query=opnborg)

[paepcke.de/opnborg](https://paepcke.de/opnborg/)

# OPNBORG 

- Selfhosted OPNSense and Unifi WebGUI portal to configure, monitor and backup [opnsense.org](https://opnsense.org/) and [ui.com](https://ui.com) appliance(s)
 
# SCREENSHOT WEBUI

![OPNBORG SAMPLE SCREENSHOT](https://github.com/paepckehh/opnborg/blob/main/resources/screenshot02.png "SCREEN")

# ⚡️QUICK START
```
OPN_TARGETS="opn01.lan,opn02.lan" OPN_APIKEY="..." OPN_APISECRET="..." go run paepcke.de/opnborg/cmd/opnborg@main
```

# ⚡️HOW TO INSTALL

```
go install paepcke.de/opnborg/cmd/opnborg@main
```

# ⚡️PRE-BUILD BINARIES (DOWNLOAD)
[https://github.com/paepckehh/opnborg/releases](https://github.com/paepckehh/opnborg/releases)

# ⚡️EXAMPLE CONFIGURATION VIA ENV 
```
please see:
- example.sh 
- example-env-config.sh
```

# ⚡️DOCKER
```
docker pull ghcr.io/paepckehh/opnborg:latest
```

# ⚡️NIXOS SYSTEM SERVICE VIA DOCKER
see opnborg-docker.nix
see opnborg-docker-complex.nix
see opnborg-prometheus-grafana.nix

```
{config, ...}: {
  ####################
  #-=# NETWORKING #=-#
  ####################
  networking = {
    firewall = {
      allowedTCPPorts = [6464]; # open tcp port 6464
    };
  };
  ########################
  #-=# VIRTUALISATION #=-#
  ########################
  virtualisation = {
    oci-containers = {
      backend = "podman";
      containers = {
        opnborg = {
          image = "ghcr.io/paepckehh/opnborg";
          volumes = ["/var/opnborg:/var/opnborg"];
          extraOptions = ["--network=host"];
          environment = {
            "OPN_PATH" = "/var/opnborg";
            "OPN_TARGETS" = "opn01.lan,opn02.lan";
            "OPN_APIKEY" = "+RIb6YWNdcDWMMM7W5ZYDkUvP4qx6e1r7e/Lg/Uh3aBH+veuWfKc7UvEELH/lajWtNxkOaOPjWR8uMcD";
            "OPN_APISECRET" = "8VbjM3HKKqQW2ozOe5PTicMXOBVi9jZTSPCGfGrHp8rW6m+TeTxHyZyAI1GjERbuzjmz6jK/usMCWR/p";
          };
        };
      };
    };
  };
}
```

# FEATURES

- Central Monitoring (version, status, online, offline, last seen, configuration compliance)
- Central Package Management (Install Packages across all OPNSenses, follow one master configuration)
- Central Configuration Audit / Change Log Trail / Backup (consolidated git repo, filesystem archive for archive & easy rapid restore)
- Central Logs Consolidation (provides internal RFC5442 syslog collector, rotate, archive, ...)
- One single binary, no dependency, cross os & hw plattform support via go (linux, freebsd, openbsd, netbsd, windows, x86, aarch64, armv7, ...) 
- Simple NixOS integration for extensive Proemetheus & Grafana (wip:wazuh,influx,greylog,...) metrics collection / monitoring / alerting
- Planned as little complementary SideKick for OPNCentral (is & will be no in-place or replacement)
- Free, Open Source, BSD License, feel free to contribute or fork


# SUPPORTED OPTIONS 

```
# Required
- OPN_APIKEY           - OPNsense Backup User APIKEY [string, base64 encoded]
- OPN_APISECRET        - OPNsense Backup User APISECRET [string, base64 encoded]
- OPN_TARGETS          - list of OPNSense Target Server to Backup [string, hostnames, comma separated] [optional: add asset-tag, via # as seprator for each host]
- OPN_TARGETS_*        - alternative: custom groups for OPNSense Target server [example: OPN_TARGETS_INTRANET="opn-int-01.lan:8443,..."], add a '#' after hostname for asset tags
- OPN_TARGETS_IMGURL_* - alternative: custom image url for customs groups within WebUI [example: OPN_TARGETS_IMGURL_INTRANET="https://paepcke.de/img/intra.png"]

# Optional
- OPN_PATH        - specify a target path (absolut or releative) to store backups [string: defaults to '.']
- OPN_TLSKEYPIN   - OPNsense TLS MitM proof Certificate Keypin [string]
- OPN_SLEEP       - daemon mode poll interval [string, defaults to 3600 seconds, minimum 5 seconds]
- OPN_EMAIL       - specify email address contact for local git commits [string: defaults to git@opnborg]
- OPN_NODAEMON    - do not run app in daemon mode, quit after one loop [bool: defaults to 'false']
- OPN_NOGIT       - do not create & update local git version repo [bool: defaults to 'false']
- OPN_GITPUSH     - push all changes to upstream git repo (needs upstream setting via .git/config)
- OPN_DEBUG       - verbose debug log mode [bool: defaults to 'false']

# PKG Installation Sync
- OPN_MASTER      - define a master server, opnborg will replicate all config changes on the master to all the hive [string, hostname]
- OPN_SYNC_PKG    - enable to unlock opnsense hive package (system/plugin) syncronisation across all targets [bool, defaults to false]

# Internal Remote Syslog Collector
- OPN_RSYSLOG_ENABLE - spin up internal RFC5424 rsyslog server, monitor all hive members log config (bool, default: false)
- OPN_RSYSLOG_SERVER - [required] define syslog srv listen ip & port [example: 192.168.0.1:5140] (Do not use 0.0.0.0, its srv & target ip conf!)

# WebConsole 
- OPN_HTTPD_DISABLE     - disable the internal httpd server (bool, default: false => not set)
- OPN_HTTPD_SERVER      - HTTPD Listen Address  [string, default: 127.0.0.1:80]
- Optional 
  - OPN_HTTPD_CACERT      - HTTPD Server CA X.509 Certificate (string: <server.pem>), defaults to <empty>, empty disables https)
  - OPN_HTTPD_CACKEY      - HTTPD Server CA Key  (string: <server.key>), defaults to <empty>, empty disables https)
  - OPN_HTTPD_CACLIENT    - HTTPD Server CA ClientCA Certificate (string: <clientCA.pem>), defaults to <empty>, if set, enforces mTLS)
  - OPN_HTTPD_COLOR_FG    - WebUI Foreground (html) color code (example: "black" or "#000000", ) 
  - OPN_HTTPD_COLOR_BG    - WebUI Background (html) color code (example: "orange" or "#ffa500")

# Prometheus 
- OPN_PROMETHEUS_WEBUI - Promometheus Web Console target & port [example: http://localhost:8443]

# Unifi 
- OPN_UNIFI_WEBUI         - Unifi Web Console target & port [example: http://localhost:8444], use '#' to add asset tag
- OPN_UNIFI_BACKUP_USER   - Unifi Backup User Account
- OPN_UNIFI_BACKUP_SECRET - Unifi Backup User Account Password
- OPN_UNIFI_BACKUP_IMGURL - Unifi Backup Group Image URL [example: export OPN_UNIFI_BACKUP_IMGURL="https://paepcke.de/img/unifi.png"]

# Unifi Inventory Export, details see: [github.com/paepckehh/uniex](https://github.com/paepckehh/uniex)
- OPN_UNIFI_EXPORT        - Unifi Inventory Nightly Exports, check into git repo  [example: export OPN_UNIFI_EXPORT='1']
- OPN_UNIFI_FORMAT        - Unifi Inventory Export formart, defaults to 'csv', optional 'json'
- OPN_UNIFI_MONGODB_URI   - Unifi Inventory Mongodb Database uri, defaults to 'mongodb://localhost:27117'

# Wazuh 
- OPN_WAZUH_WEBUI - Wazuh Web Console target & port [example: http://localhost:8446]

# Grafana
- OPN_GRAFANA_WEBUI             - grafana web console target & port [example: http://localhost:8446]
- OPN_GRAFANA_DASHBOARD_FREEBSD - grafana freebsd node dashboard id / dashboard name (example: Kczn-jPZz/node-exporter-freebsd)
- OPN_GRAFANA_DASHBOARD_HAPROXY - grafana haproxy node dashboard id / dashboard name (example: P4zs3-ces/haproxy-2-full)
- OPN_GRAFANA_DASHBOARD_UNIFI   - grafana unpoller dashboard id / dashboard name (example: g3kd0-3ds/unpoller)

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
- OPN_TARGETS & OPN_MASTER must hold the (reachable) WebUI Interface(s) [example: 192.168.0.1] including the port, if not 443 (example: 192.168.0.1:8443)
- Clear text HTTP protocol is not supported, switch on HTTPS for your admin interface (self-signed certificates will do)
- ATT: HTTPS chain verification via system os trust store(s) is disabled by default: use OPN_TLSKEYPIN (!!!)
```
# NIXOS: PROMETHEUS AND GRAFANA INTEGRATION

```
If you run OPNBORG on NixOS
- adapt target IPs and import opnborg-prometheus-grafana.nix via

  imports = [
    ./opnborg-prometheus-grafana-wazuh.nix
  ];

- import into your grafana instance this dashboards 
- set OPN_GRAFANA_DASHBOARD_*='id/names' after import
    - [FreeBSD Node Exporter](https://github.com/rfmoz/grafana-dashboards/blob/master/prometheus/node-exporter-freebsd.json)
    - [Linux Node Exporter](https://github.com/rfmoz/grafana-dashboards/blob/master/prometheus/node-exporter-full.json)
    - [HAProxy2](https://github.com/rfmoz/grafana-dashboards/blob/master/prometheus/haproxy-2-full.json)

todo:
- add wazuh
- add pre-configured optimised opnsense dashboards
- opnborg nixpkg and declarative systemd service ( services.opnborg = { enable = true; } ... )
```

# DOCS

[pkg.go.dev/paepcke.de/opnborg](https://pkg.go.dev/paepcke.de/opnborg)

# 🛡 License

[![License](https://img.shields.io/github/license/paepckehh/opnborg)](https://github.com/paepckehh/opnborg/blob/master/LICENSE)

This project is licensed under the terms of the `BSD 3-Clause License` license. See [LICENSE](https://github.com/paepckehh/opnborg/blob/master/LICENSE) for more details.

# 📃 Citation

```bibtex
@misc{opnborg,
  author = {Michael Paepcke},
  title = {Selfhost-able OPNSense Appliance Configuration Managenment & Backup Portal},
  year = {2024},
  publisher = {GitHub},
  journal = {GitHub repository},
  howpublished = {\url{https://paepcke.de/opnborg}}
}
```

# CONTRIBUTION

Yes, Please! PRs Welcome! 

# SPONSORS & SPECIAL THANKS

- [pvz.digital](https://pvz.digital)
- UX Borg Design & Contrib: [@Codebase-Torben](https://github.com/Codebase-Torben) & [@Jones71190](https://github.com/Jones71190)
