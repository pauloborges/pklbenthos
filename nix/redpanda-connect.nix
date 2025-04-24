# The command of Redpanda Connect, from the tarball that each release
# publishes.
#
# nixpkgs holds no package for it. It holds `redpanda-client`, which gives
# `rpk`, and `rpk connect` downloads a copy of this command at run time. A
# derivation keeps the version in the lock file instead, so every shell of the
# project runs the same one.
{
  lib,
  stdenv,
  fetchurl,
}:
let
  version = "4.104.0";

  # The checksum of each tarball comes from the ".sha256" asset beside it in
  # the release.
  sources = {
    aarch64-darwin = {
      platform = "darwin_arm64";
      hash = "971d9c5d13b74456aec94b49bad61b63644a604ea3c2c495ab00f74e24d32d36";
    };
    x86_64-darwin = {
      platform = "darwin_amd64";
      hash = "a95bf277919e922f0ebbdb9621b142f428e1dbe9991cf20f72af5583366c5aa6";
    };
    aarch64-linux = {
      platform = "linux_arm64";
      hash = "99e8228ea61b42a4fc4243721b59200dc63f6a9a787a7a94c66cba6c83084297";
    };
    x86_64-linux = {
      platform = "linux_amd64";
      hash = "9da44d4f27d6ea292d41c747550f0211c9ce7487ff9692720756ff0e36965592";
    };
  };

  source =
    sources.${stdenv.hostPlatform.system}
      or (throw "redpanda-connect: the release publishes no binary for ${stdenv.hostPlatform.system}");
in
stdenv.mkDerivation {
  pname = "redpanda-connect";
  inherit version;

  src = fetchurl {
    url = "https://github.com/redpanda-data/connect/releases/download/v${version}/redpanda-connect_${version}_${source.platform}.tar.gz";
    sha256 = source.hash;
  };

  # The tarball holds the command, a changelog and the licenses at its root,
  # and no directory above them.
  sourceRoot = ".";

  dontConfigure = true;
  dontBuild = true;

  # The command is a Go binary that carries no dynamic library, and a strip
  # breaks the signature that macOS reads.
  dontStrip = true;

  installPhase = ''
    runHook preInstall

    install -Dm755 redpanda-connect $out/bin/redpanda-connect

    runHook postInstall
  '';

  meta = {
    description = "Stream processor of Redpanda Connect";
    homepage = "https://github.com/redpanda-data/connect";

    # The tarball holds the enterprise components as well as the FOSS ones.
    # The enterprise components are under the Redpanda Community License, which
    # gives the source but keeps conditions on the use.
    license = lib.licenses.unfree;

    mainProgram = "redpanda-connect";
    platforms = lib.attrNames sources;
  };
}
