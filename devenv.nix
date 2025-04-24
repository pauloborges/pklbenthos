{ pkgs, ... }:

let
  redpanda-connect = pkgs.callPackage ./nix/redpanda-connect.nix { };
  pkldoc = pkgs.callPackage ./nix/pkldoc.nix { };
in
{
  languages.go.enable = true;

  packages = [
    pkgs.pkl

    # The pkl command holds no `doc` subcommand, so Pkldoc comes on its own.
    # See nix/pkldoc.nix.
    pkldoc

    # nixpkgs holds no package for the command of Redpanda Connect, so this
    # repository carries a derivation of its own. See nix/redpanda-connect.nix.
    redpanda-connect

    pkgs.golangci-lint
    pkgs.gotools
  ];

  # Build every command under ./cmd into ./bin.
  scripts.build.exec = ''
    cd "$DEVENV_ROOT" && go build -o bin/ ./cmd/...
  '';

  # Named `check`, not `test`, because `test` is a shell builtin and a
  # builtin always wins over a command in PATH.
  scripts.check.exec = ''
    cd "$DEVENV_ROOT" && go test ./...
  '';

  # Run the command from source. It takes the arguments of the command itself.
  #
  # Only the build step moves to the root of the repository, so the command
  # keeps the working directory of the caller. A relative path in an argument
  # then points where the caller expects. `go run` cannot do this, because it
  # needs the module directory as the working directory, and the program that
  # it starts inherits that directory.
  #
  # The build is incremental, so an unchanged tree costs about a second.
  scripts.pklbenthos.exec = ''
    (cd "$DEVENV_ROOT" && go build -o bin/pklbenthos ./cmd/pklbenthos) || exit 1

    exec "$DEVENV_ROOT"/bin/pklbenthos "$@"
  '';

  enterShell = ''
    echo "pklbenthos development environment"
    echo "  go:  $(go version | cut -d' ' -f3)"
    echo "  pkl: $(pkl --version | cut -d' ' -f2)"
    echo "  pkldoc: ${pkldoc.version}"
    echo "  redpanda-connect: ${redpanda-connect.version}"
    echo
    echo "Commands: build, check, pklbenthos"
  '';

  enterTest = ''
    go build ./...
    go test ./...
  '';

  git-hooks.hooks = {
    gofmt.enable = true;
    govet.enable = true;
  };
}
