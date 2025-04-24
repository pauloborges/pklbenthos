# Pkldoc, the documentation generator of Pkl.
{
  lib,
  stdenv,
  fetchurl,
}:
let
  # Keep this in step with the version of pkgs.pkl in devenv.nix. A site that
  # one version writes is not the site that another one reads.
  version = "0.31.1";

  sources = {
    aarch64-darwin = {
      platform = "macos-aarch64";
      hash = "c31c360f7af58c6c94bbc2990f6c5b7be198b508b0a6eeb9370378598795932a";
    };
    x86_64-darwin = {
      platform = "macos-amd64";
      hash = "f56e1983ea2e37b12522bb4f16ad19647bb81e419711fcff6ffbcadbcf7d09b9";
    };
    aarch64-linux = {
      platform = "linux-aarch64";
      hash = "5e8b98e90a17ae367ae3328136808bb6753aec2b7cd6fadb11bccc91295b4a93";
    };
    x86_64-linux = {
      platform = "linux-amd64";
      hash = "f640f141e109d2d079832940c7a7b8997aad46909103f8d9c3b14c2a40f3881c";
    };
  };

  source =
    sources.${stdenv.hostPlatform.system}
      or (throw "pkldoc: the release publishes no binary for ${stdenv.hostPlatform.system}");
in
stdenv.mkDerivation {
  pname = "pkldoc";
  inherit version;

  src = fetchurl {
    url = "https://github.com/apple/pkl/releases/download/${version}/pkldoc-${source.platform}";
    sha256 = source.hash;
  };

  # The release publishes the command itself, and no archive around it.
  dontUnpack = true;
  dontConfigure = true;
  dontBuild = true;

  # A native image of GraalVM carries its own layout, and a strip breaks it.
  dontStrip = true;

  installPhase = ''
    runHook preInstall

    install -Dm755 $src $out/bin/pkldoc

    runHook postInstall
  '';

  meta = {
    description = "Documentation generator of Pkl";
    homepage = "https://pkl-lang.org/main/current/pkl-doc/index.html";
    license = lib.licenses.asl20;
    mainProgram = "pkldoc";
    platforms = lib.attrNames sources;
  };
}
