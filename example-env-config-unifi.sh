#!/bin/sh
# export OPN_TARGETS='opn00.lan:8443,opn01.lan:8443#RACK-PROD01,opn02.lan:8443#RACK-PROD02'
export OPN_TARGETS_STANDBY='opn00.lan:8443#RACK-LAB-2ND-FLOOR'
export OPN_TARGETS_INTRANET='opn01.lan:8443#RACK-PROD01,opn02.lan:8443#RACK-PROD02'
export OPN_TARGETS_EXTERNAL='opn03.lan:8443#RACK-DMZ01-VODAFONE,opn04.lan:8443#RACK-DMZ02-TELEKOM'
export OPN_TARGETS_IMGURL_STANDBY='https://paepcke.de/res/hot.png'
export OPN_TARGETS_IMGURL_INTRANET='https://paepcke.de/res/int.png'
export OPN_TARGETS_IMGURL_EXTERNAL='https://paepcke.de/res/ext.png'
export OPN_MASTER='opn01.lan:8443'
export OPN_APIKEY='+RIb6YWNdcDWMMM7W5ZYDkUvP4qx6e1r7e/Lg/Uh3aBH+veuWfKc7UvEELH/lajWtNxkOaOPjWR8uMcD'
export OPN_APISECRET='8VbjM3HKKqQW2ozOe5PTicMXOBVi9jZTSPCGfGrHp8rW6m+TeTxHyZyAI1GjERbuzjmz6jK/usMCWR/p'
export OPN_TLSKEYPIN='SG95BZoovDVQtclwEhINMitua05ZP9NfuI0mzzj0fXI='
export OPN_PATH='/tmp/opn'
export OPN_SLEEP='60'
export OPN_DEBUG='1'
export OPN_SYNC_PKG='1'
export OPN_RSYSLOG_ENABLE='1'
export OPN_RSYSLOG_SERVER='192.168.122.1:5140'
export OPN_HTTPD_SERVER='127.0.0.1:6464'
export OPN_GRAFANA_WEBUI='http://localhost:9090'
export OPN_GRAFANA_DASHBOARD_FREEBSD='Kczn-jPZz/node-exporter-freebsd'
export OPN_GRAFANA_DASHBOARD_HAPROXY='rEqu1u5ue/haproxy-2-full'
export OPN_GRAFANA_DASHBOARD_UNIFI='rEqu1u5ue/haproxy-2-full'
export OPN_WAZUH_WEBUI='http://localhost:9292'
export OPN_PROMETHEUS_WEBUI='http://localhost:9191'
export OPN_UNIFI_WEBUI='https://localhost:8443#RACK-PROD03'
export OPN_UNIFI_VERSION='8.5.6'
export OPN_UNIFI_BACKUP_USER='admin'
export OPN_UNIFI_BACKUP_SECRET='start'
export OPN_UNIFI_BACKUP_IMGURL='https://paepcke.de/res/uni.png'
#export OPN_UNIFI_EXPORT='1'
export OPN_UNIFI_FORMAT='csv'
export OPN_UNIFI_MONGODB_URI='mongodb://127.0.0.1:27117'
export OPN_GITSRV='1'
