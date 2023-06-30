{
  perSystem = {
    self',
    lib,
    pkgs,
    ...
  }: let
    config = pkgs.writeTextFile {
      name = "nats.conf";
      text = ''
        ## Default NATS server configuration (see: https://docs.nats.io/running-a-nats-service/configuration)

        ## Host for client connections.
        host: "127.0.0.1"

        ## Port for client connections.
        port: 4222

        ## Port for monitoring
        http_port: 8222

        ## Configuration map for JetStream.
        ## see: https://docs.nats.io/running-a-nats-service/configuration#jetstream
        jetstream {}
      '';
    };
  in {
    config.devshells.default = {
      env = [
        {
          name = "NATS_HOME";
          eval = "$PRJ_DATA_DIR/nats";
        }
      ];

      devshell.startup = {
        setup-datadir.text = "mkdir -p $PRJ_DATA_DIR";
        setup-nats = {
          deps = ["setup-datadir"];
          text = ''
            mkdir -p $NATS_HOME
            cp ${config} "$NATS_HOME/nats.conf"
          '';
        };
      };

      commands = [
        {
          category = "development";
          help = "run local dev services";
          package = self'.packages.dev;
        }
        {
          category = "development";
          help = "re-initialise data directory";
          name = "dev-init";
          command = "rm -rf $PRJ_DATA_DIR && direnv reload";
        }
      ];
    };

    config.process-compose.dev.settings = {
      log_location = "$PRJ_DATA_DIR/dev.log";

      processes = {
        nats-server = {
          working_dir = "$NATS_HOME";
          command = ''${lib.getExe pkgs.nats-server} -c ./nats.conf -sd ./'';
          readiness_probe = {
            http_get = {
              host = "127.0.0.1";
              port = 8222;
              path = "/healthz";
            };
            initial_delay_seconds = 2;
          };
        };
      };
    };
  };
}
