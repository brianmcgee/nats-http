{
  perSystem = {
    lib,
    pkgs,
    ...
  }: {
    packages.tests = pkgs.buildGoModule rec {
      pname = "nats.http";
      version = "0.0.1+dev";

      src = ../.;
      vendorSha256 = "sha256-bbACDGInMCjQjO/ST0Ty2aI3GhHkSTQqTPl6OqLVI1c=";

      postInstall = ''
        # run test coverage
        mkdir -p $out/share/test
        go test --race -covermode=atomic -coverprofile=$out/share/test/coverage.out -v ./...
      '';

      ldflags = [
        "-X 'build.Name=${pname}'"
        "-X 'build.Version=${version}'"
      ];
    };
  };
}
