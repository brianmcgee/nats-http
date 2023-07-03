{
  self,
  lib,
  ...
}: {
  perSystem = {
    self',
    inputs',
    pkgs,
    config,
    ...
  }: {
    checks = {
      statix =
        pkgs.runCommand "statix" {
          nativeBuildInputs = [pkgs.statix];
        } ''
          cp --no-preserve=mode -r ${self} source
          cd source
          HOME=$TMPDIR statix check
          touch $out
        '';
      # mixin the tests package
      inherit (self'.packages) tests;
    };

    devshells.default = {
      commands = [
        {
          category = "build";
          name = "check";
          help = "run all linters and build all packages";
          command = "nix flake check";
        }
      ];
    };
  };
}
