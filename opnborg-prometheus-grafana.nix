{config, ...}: {
  # example nixos config for opn01.lan & opn02.lan including prometheus & grafana
  # WebUI http://localhost:6464
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
            "OPN_TARGETS" = "opn01.lan,opn02.lan";
            "OPN_APIKEY" = "+RIb6YWNdcDWMMM7W5ZYDkUvP4qx6e1r7e/Lg/Uh3aBH+veuWfKc7UvEELH/lajWtNxkOaOPjWR8uMcD";
            "OPN_APISECRET" = "8VbjM3HKKqQW2ozOe5PTicMXOBVi9jZTSPCGfGrHp8rW6m+TeTxHyZyAI1GjERbuzjmz6jK/usMCWR/p";
          };
        };
      };
    };
  };
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
                "opn01.lan:9100" # example opnsense node IP
                "opn02.lan:9100" # example opnsense node IP
              ];
            }
          ];
        }
        {
          job_name = "haproxy";
          static_configs = [
            {
              targets = [
                "opn01.lan:8404" # example opnsense node IP
                "opn02.lan:8404" # example opnsense node IP
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
