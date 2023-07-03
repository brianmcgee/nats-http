{
  perSystem = {
    lib,
    pkgs,
    ...
  }: {
    packages.default = pkgs.buildGoModule rec {
      pname = "nats.http";
      version = "0.0.1+dev";

      src = lib.cleanSourceAndNix ../.;
      vendorSha256 = "sha256-bbACDGInMCjQjO/ST0Ty2aI3GhHkSTQqTPl6OqLVI1c=";

      ldflags = [
        "-X 'build.Name=${pname}'"
        "-X 'build.Version=${version}'"
      ];

      meta = with lib; {
        maintainers = [
          "Brian McGee <brian@bmcgee.ie>"
        ];
        description = "Nats Http Transport";
        homepage = "https://github.com/brianmcgee/nats.http";
        license = licenses.mit;
      };
    };
  };
}
