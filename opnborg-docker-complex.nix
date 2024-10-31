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
          extraOptions = ["--network=host"];
          environment = {
            "OPN_APIKEY" = "+RIb6YWNdcDWMMM7W5ZYDkUvP4qx6e1r7e/Lg/Uh3aBH+veuWfKc7UvEELH/lajWtNxkOaOPjWR8uMcD";
            "OPN_APISECRET" = "8VbjM3HKKqQW2ozOe5PTicMXOBVi9jZTSPCGfGrHp8rW6m+TeTxHyZyAI1GjERbuzjmz6jK/usMCWR/p";
            "OPN_TLSKEYPIN" = "8VbjM3HKKqQW2ozOe5PTicMXOBVi9jZTSPCGfGrHp8rW6m+TeTxHyZyAI1GjERbuzjmz6jK/usMCWR/p";
            "OPN_MASTER" = "opn00.lan:8443";
            "OPN_TARGETS_HOTSTANDBY" = "opn00.lan:8443";
            "OPN_TARGETS_PRODUCTION" = "opn01.lan:8443,opn02.lan:8443";
            "OPN_TARGETS_IMGURL_HOTSTANDBY" = "https://icon-library.com/images/freebsd-icon/freebsd-icon-16.jpg";
            "OPN_TARGETS_IMGURL_PRODUCTION" = "https://icon-library.com/images/freebsd-icon/freebsd-icon-16.jpg";
            "OPN_SLEEP" = "60";
            "OPN_DEBUG" = "true";
            "OPN_SYNC_PKG" = "true";
            "OPN_HTTPD_ENABLE" = "true";
            "OPN_HTTPD_SERVER" = "127.0.0.1:6464";
            "OPN_HTTPD_COLOR_FG" = "white";
            "OPN_HTTPD_COLOR_BG" = "grey";
            "OPN_RSYSLOG_ENABLE" = "true";
            "OPN_RSYSLOG_SERVER" = "192.168.122.1:5140";
            "OPN_GRAFANA_WEBUI" = "http://localhost:9090";
            "OPN_GRAFANA_DASHBOARD_FREEBSD" = "Kczn-jPZz/node-exporter-freebsd";
            "OPN_GRAFANA_DASHBOARD_HAPROXY" = "rEqu1u5ue/haproxy-2-full";
            "OPN_WAZUH_WEBUI" = "http://localhost:9292";
            "OPN_PROMETHEUS_WEBUI" = "http://localhost:9191";
          };
        };
      };
    };
  };
}
