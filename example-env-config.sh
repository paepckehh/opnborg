#!/bin/sh
# export OPN_TARGETS='opn01.lan:8443,opn02.lan:8443,opn03.lan:8443,opn04.lan:8443,opn05.lan:8443'
export OPN_TARGETS_HOTSTANDBY='opn00.lan:8443'
export OPN_TARGETS_IMGURL_HOTSTANDBY='https://avatars.githubusercontent.com/u/120342602?s=96&v=4'
export OPN_TARGETS_PRODUCTION='opn01.lan:8443,opn02.lan:8443'
export OPN_MASTER='opn01.lan:8443'
export OPN_APIKEY='+RIb6YWNdcDWMMM7W5ZYDkUvP4qx6e1r7e/Lg/Uh3aBH+veuWfKc7UvEELH/lajWtNxkOaOPjWR8uMcD'
export OPN_APISECRET='8VbjM3HKKqQW2ozOe5PTicMXOBVi9jZTSPCGfGrHp8rW6m+TeTxHyZyAI1GjERbuzjmz6jK/usMCWR/p'
export OPN_TLSKEYPIN='SG95BZoovDVQtclwEhINMitua05ZP9NfuI0mzzj0fXI='
export OPN_PATH='/tmp/opn'
export OPN_SLEEP='60'
export OPN_DEBUG='true'
export OPN_SYNC_PKG='true'
export OPN_HTTPD_SERVER='127.0.0.1:6464'
export OPN_HTTPD_COLOR_FG='white'
export OPN_HTTPD_COLOR_BG='grey'
export OPN_RSYSLOG_ENABLE='true'
export OPN_RSYSLOG_SERVER='192.168.122.1:5140'
export OPN_GRAFANA_WEBUI='http://localhost:9090'
export OPN_GRAFANA_DASHBOARD_FREEBSD='Kczn-jPZz/node-exporter-freebsd'
export OPN_GRAFANA_DASHBOARD_HAPROXY='rEqu1u5ue/haproxy-2-full'
export OPN_WAZUH_WEBUI='http://localhost:9292'
export OPN_PROMETHEUS_WEBUI='http://localhost:9191'
