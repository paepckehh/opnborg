{config, ...}: {
  ##################
  #-=# SERVICES #=-#
  ##################
  services = {
    prometheus = {
      enable = true;
      alertmanager.port = 9292;
      port = 9191;
      retentionTime = "365d";
      scrapeConfigs = [
        {
          job_name = "node";
          static_configs = [
            {
              targets = [
                "127.0.0.1:${toString config.services.prometheus.exporters.node.port}" # self
                "192.168.122.2:9100" # example opnsense node IP
                "192.168.122.3:9100" # example opnsense node IP
              ];
            }
          ];
        }
        {
          job_name = "haproxy";
          static_configs = [
            {
              targets = [
                "192.168.122.2:8404" # example opnsense node IP
                "192.168.122.3:8404" # example opnsense node IP
              ];
            }
          ];
        }
      ];
      exporters.node = {
        enable = true;
        port = 9100;
        enabledCollectors = [
          "logind"
          "systemd"
        ];
        disabledCollectors = [];
        openFirewall = true;
      };
    };
    grafana = {
      enable = true;
      settings = {
        server = {
          http_addr = "127.0.0.1";
          http_port = 9090;
          domain = "localhost";
        };
      };
    };
  };
}
