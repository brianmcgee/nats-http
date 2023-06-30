{inputs, ...}: {
  imports = [
    inputs.flake-root.flakeModule
    ./checks.nix
    ./dev.nix
    ./formatter.nix
    ./shell.nix
  ];
}
