# Paquets connus

[← apt](apt.md)

<!-- Fichier généré depuis internal/apt (aliases.go, tools.go, resolve.go) : ne pas modifier à la main.
     Pour le régénérer : UPDATE_DOCS=1 go test ./internal/apt -run TestPackagesDoc -->

`apt install` accepte les noms Linux (Debian, Ubuntu, Arch, Homebrew) de cette page. Les identifiants winget
ont été vérifiés dans le dépôt des manifestes winget. Pour un nom absent, `apt` cherche dans winget le logiciel
qui porte ce nom court ou fournit cette commande, et te fait choisir si plusieurs correspondent : voir
[apt › Noms de paquets](apt.md#noms-de-paquets).

265 noms pour 144 logiciels, plus les outils ci-dessous.

## Installés par apt lui-même

Ces outils ne sont pas dans winget (ou demandent un réglage que winget ne fait pas seul). `apt` les installe
depuis leur source officielle. Les outils de développement sont rangés dans `%LOCALAPPDATA%\OrvaenTerminal\outils`,
et leur dossier est ajouté à ton PATH. `apt upgrade` les met à jour, `apt remove` les désinstalle.

| Noms | Ce qui est installé | Source | A besoin de |
|---|---|---|---|
| `docker`, `docker.io`, `docker-ce`, `docker-compose` | Docker Engine et Compose, sans Docker Desktop | [Docker léger](docker.md) | WSL2 |
| `terminal` (avec `apt upgrade`) | Orvaen Terminal | releases GitHub, somme SHA-256 vérifiée | |
| `composer`, `php-composer` | Composer | getcomposer.org, somme SHA-256 vérifiée | `php` |
| `maven`, `mvn`, `apache-maven` | Maven | Apache (dernière version stable), somme SHA-512 vérifiée | `java` |
| `gradle` | Gradle | services.gradle.org, somme SHA-256 vérifiée | `java` |
| `poetry`, `python3-poetry`, `python-poetry` | Poetry | installeur officiel install.python-poetry.org | `python` |
| `build-essential`, `buildtools`, `msvc`, `visual-studio-build-tools`, `visualcpp-build-tools`, `vs-buildtools` | Compilateur C/C++ MSVC : `Microsoft.VisualStudio.BuildTools` avec la charge de travail « Développement Desktop en C++ » et les bibliothèques Spectre-mitigated | winget | |
| `msvc-spectre`, `spectre`, `spectre-libs`, `vc-spectre` | Bibliothèques Spectre-mitigated, ajoutées à un MSVC déjà installé | installeur de Visual Studio, en ligne de commande | MSVC (sinon installé avec) |

## Fournis avec un autre logiciel

Ces outils ne s'installent pas seuls : `apt install` installe le logiciel qui les fournit, `apt remove` refuse (pour ne pas désinstaller tout le logiciel), et `apt upgrade` les met à jour par leur propre commande quand elle existe.

| Outil | Fourni avec | apt upgrade lance |
|---|---|---|
| `cargo` | Rust (rustup) (`apt install rust`) | `rustup update` |
| `clang-format` | LLVM (`apt install llvm`) | la mise à jour de `llvm` |
| `corepack` | Node.js (`apt install node`) | la mise à jour de `node` |
| `dot` | Graphviz (`apt install graphviz`) | la mise à jour de `graphviz` |
| `g++` | GCC (WinLibs) (`apt install gcc`) | la mise à jour de `gcc` |
| `gdb` | GCC (WinLibs) (`apt install gcc`) | la mise à jour de `gcc` |
| `javac` | le JDK (`apt install java`) | la mise à jour de `java` |
| `lld` | LLVM (`apt install llvm`) | la mise à jour de `llvm` |
| `npm` | Node.js (`apt install node`) | `npm install --global npm@latest` |
| `npx` | Node.js (`apt install node`) | la mise à jour de `node` |
| `pip` | Python (`apt install python`) | `python -m pip install --upgrade pip` |
| `pip3` | Python (`apt install python`) | `python -m pip install --upgrade pip` |
| `python3-pip` | Python (`apt install python`) | `python -m pip install --upgrade pip` |
| `rustc` | Rust (rustup) (`apt install rust`) | `rustup update` |

## Déjà dans Windows

Pour ces paquets, `apt` explique l'équivalent au lieu d'installer autre chose.

| Nom | Ce qu'apt répond |
|---|---|
| `openssh` | OpenSSH est déjà intégré à Windows : ssh, scp et sftp marchent tout de suite. |
| `openssh-client` | OpenSSH est déjà intégré à Windows : ssh, scp et sftp marchent tout de suite. |
| `ssh` | OpenSSH est déjà intégré à Windows : ssh, scp et sftp marchent tout de suite. |
| `sudo` | sudo est déjà intégré au terminal. |
| `tar` | tar est déjà intégré à Windows. |
| `tree` | tree est déjà intégré à Windows. |
| `unzip` | Windows ouvre les .zip tout seul ; en ligne de commande : tar -xf fichier.zip. |
| `zip` | en ligne de commande : tar -a -cf archive.zip dossier (intégré à Windows), ou apt install 7zip. |

## Éditeurs et IDE

| Noms | Identifiant winget |
|---|---|
| `code`, `vscode`, `visual-studio-code`, `visual-studio-code-bin` | `Microsoft.VisualStudioCode` |
| `code-insiders`, `visual-studio-code-insiders` | `Microsoft.VisualStudioCode.Insiders` |
| `neovim`, `nvim` | `Neovim.Neovim` |
| `vim`, `gvim`, `vim-gtk3` | `vim.vim` |
| `nano` | `GNU.Nano` |
| `emacs` | `GNU.Emacs` |
| `helix`, `hx` | `Helix.Helix` |
| `sublime-text`, `sublime-text-4`, `subl` | `SublimeHQ.SublimeText.4` |
| `notepad++`, `notepadplusplus` | `Notepad++.Notepad++` |
| `android-studio` | `Google.AndroidStudio` |
| `intellij-idea-community`, `intellij-idea-community-edition`, `idea` | `JetBrains.IntelliJIDEA.Community` |
| `pycharm-community`, `pycharm-community-edition` | `JetBrains.PyCharm.Community` |
| `obsidian` | `Obsidian.Obsidian` |

## Langages et outils de développement

| Noms | Identifiant winget |
|---|---|
| `git`, `git-all` | `Git.Git` |
| `gh`, `github-cli` | `GitHub.cli` |
| `glab` | `GLab.GLab` |
| `github-desktop` | `GitHub.GitHubDesktop` |
| `lazygit` | `JesseDuffield.lazygit` |
| `git-delta`, `delta` | `dandavison.delta` |
| `node`, `nodejs`, `npm`, `nodejs-lts` | `OpenJS.NodeJS.LTS` |
| `node-current`, `nodejs-current` | `OpenJS.NodeJS` |
| `nvm`, `nvm-windows` | `CoreyButler.NVMforWindows` |
| `yarn` | `Yarn.Yarn` |
| `pnpm` | `pnpm.pnpm` |
| `bun` | `Oven-sh.Bun` |
| `deno` | `DenoLand.Deno` |
| `python`, `python3`, `python-is-python3`, `python3-pip`, `pip`, `python3-full` | `Python.Python.3.14` |
| `py`, `python-launcher` | `Python.Launcher` |
| `uv` | `astral-sh.uv` |
| `ruff` | `astral-sh.ruff` |
| `miniconda`, `miniconda3`, `conda` | `Anaconda.Miniconda3` |
| `go`, `golang`, `golang-go` | `GoLang.Go` |
| `rust`, `rustup`, `rustc`, `cargo` | `Rustlang.Rustup` |
| `zig` | `zig.zig` |
| `llvm`, `clang`, `clang-format`, `lld` | `LLVM.LLVM` |
| `gcc`, `g++`, `gdb`, `mingw`, `mingw-w64`, `gcc-mingw-w64`, `winlibs` | `BrechtSanders.WinLibs.POSIX.UCRT` |
| `msys2` | `MSYS2.MSYS2` |
| `make` | `ezwinports.make` |
| `cmake` | `Kitware.CMake` |
| `java`, `jdk`, `default-jdk`, `default-jre`, `openjdk`, `openjdk-25-jdk`, `temurin`, `jdk25` | `EclipseAdoptium.Temurin.25.JDK` |
| `openjdk-21-jdk`, `openjdk-21-jre`, `jdk21`, `openjdk21`, `jdk-openjdk21` | `EclipseAdoptium.Temurin.21.JDK` |
| `openjdk-17-jdk`, `openjdk-17-jre`, `jdk17`, `openjdk17` | `EclipseAdoptium.Temurin.17.JDK` |
| `dotnet`, `dotnet-sdk`, `dotnet-sdk-10.0` | `Microsoft.DotNet.SDK.10` |
| `dotnet-sdk-9.0` | `Microsoft.DotNet.SDK.9` |
| `dotnet-sdk-8.0` | `Microsoft.DotNet.SDK.8` |
| `php`, `php8.5`, `php-cli` | `PHP.PHP.8.5` |
| `php8.4` | `PHP.PHP.8.4` |
| `ruby`, `ruby-full`, `rubygems` | `RubyInstallerTeam.RubyWithDevKit.3.4` |
| `postgresql`, `postgres`, `psql`, `postgresql-18` | `PostgreSQL.PostgreSQL.18` |
| `postgresql-17` | `PostgreSQL.PostgreSQL.17` |
| `mysql`, `mysql-server` | `Oracle.MySQL` |
| `mongodb`, `mongod`, `mongodb-server` | `MongoDB.Server` |
| `redis`, `redis-server` | `Redis.Redis` |
| `sqlite`, `sqlite3` | `SQLite.SQLite` |
| `postman` | `Postman.Postman` |
| `graphviz`, `dot` | `Graphviz.Graphviz` |
| `pandoc` | `JohnMacFarlane.Pandoc` |
| `hugo`, `hugo-extended` | `Hugo.Hugo.Extended` |
| `caddy` | `CaddyServer.Caddy` |
| `ollama` | `Ollama.Ollama` |

## Conteneurs et cloud

| Noms | Identifiant winget |
|---|---|
| `docker-desktop` | `Docker.DockerDesktop` |
| `podman` | `RedHat.Podman` |
| `podman-desktop` | `RedHat.Podman-Desktop` |
| `rancher-desktop` | `SUSE.RancherDesktop` |
| `kubectl`, `kubernetes-cli`, `kubernetes-client` | `Kubernetes.kubectl` |
| `minikube` | `Kubernetes.minikube` |
| `kind` | `Kubernetes.kind` |
| `helm` | `Helm.Helm` |
| `k3d` | `k3d.k3d` |
| `terraform` | `Hashicorp.Terraform` |
| `awscli`, `aws-cli`, `aws` | `Amazon.AWSCLI` |
| `azure-cli`, `az` | `Microsoft.AzureCLI` |
| `wsl` | `Microsoft.WSL` |

## Ligne de commande

| Noms | Identifiant winget |
|---|---|
| `coreutils`, `uutils-coreutils` | `Microsoft.Coreutils` |
| `curl` | `cURL.cURL` |
| `wget` | `JernejSimoncic.Wget` |
| `jq` | `jqlang.jq` |
| `ripgrep`, `rg` | `BurntSushi.ripgrep.MSVC` |
| `fd`, `fd-find` | `sharkdp.fd` |
| `bat`, `batcat` | `sharkdp.bat` |
| `fzf` | `junegunn.fzf` |
| `eza`, `exa` | `eza-community.eza` |
| `zoxide` | `ajeetdsouza.zoxide` |
| `starship` | `Starship.Starship` |
| `btop`, `htop` | `aristocratos.btop4win` |
| `7zip`, `p7zip`, `p7zip-full`, `7z`, `7-zip` | `7zip.7zip` |
| `ffmpeg` | `Gyan.FFmpeg` |
| `imagemagick`, `magick` | `ImageMagick.ImageMagick` |
| `pwsh`, `powershell` | `Microsoft.PowerShell` |
| `rclone` | `Rclone.Rclone` |
| `nmap` | `Insecure.Nmap` |
| `wezterm` | `wez.wezterm` |
| `alacritty` | `Alacritty.Alacritty` |
| `terminal`, `windows-terminal` | `Microsoft.WindowsTerminal` |
| `powertoys` | `Microsoft.PowerToys` |

## Applications

| Noms | Identifiant winget |
|---|---|
| `chrome`, `google-chrome`, `google-chrome-stable` | `Google.Chrome` |
| `chromium`, `chromium-browser`, `ungoogled-chromium` | `Hibbiki.Chromium` |
| `firefox` | `Mozilla.Firefox` |
| `sccache` | `Mozilla.sccache` |
| `firefox-esr` | `Mozilla.Firefox.ESR` |
| `brave`, `brave-browser` | `Brave.Brave` |
| `vivaldi`, `vivaldi-stable` | `Vivaldi.Vivaldi` |
| `opera` | `Opera.Opera` |
| `edge`, `microsoft-edge`, `microsoft-edge-stable` | `Microsoft.Edge` |
| `thunderbird` | `Mozilla.Thunderbird` |
| `discord` | `Discord.Discord` |
| `slack`, `slack-desktop` | `SlackTechnologies.Slack` |
| `telegram`, `telegram-desktop` | `Telegram.TelegramDesktop` |
| `zoom` | `Zoom.Zoom` |
| `teams`, `microsoft-teams` | `Microsoft.Teams` |
| `spotify`, `spotify-client` | `Spotify.Spotify` |
| `vlc` | `VideoLAN.VLC` |
| `mpv` | `mpv.net` |
| `obs`, `obs-studio` | `OBSProject.OBSStudio` |
| `gimp` | `GIMP.GIMP.3` |
| `inkscape` | `Inkscape.Inkscape` |
| `blender` | `BlenderFoundation.Blender` |
| `audacity` | `Audacity.Audacity` |
| `handbrake` | `HandBrake.HandBrake` |
| `libreoffice`, `libreoffice-fresh`, `libreoffice-still` | `TheDocumentFoundation.LibreOffice` |
| `keepassxc` | `KeePassXCTeam.KeePassXC` |
| `bitwarden` | `Bitwarden.Bitwarden` |
| `qbittorrent` | `qBittorrent.qBittorrent` |
| `steam`, `steam-installer` | `Valve.Steam` |
| `wireshark`, `wireshark-qt` | `WiresharkFoundation.Wireshark` |
| `putty` | `PuTTY.PuTTY` |
| `winscp` | `WinSCP.WinSCP` |
| `wireguard`, `wireguard-tools` | `WireGuard.WireGuard` |
| `openvpn` | `OpenVPNTechnologies.OpenVPN` |
| `tailscale` | `Tailscale.Tailscale` |
| `syncthing` | `Syncthing.Syncthing` |
| `miktex`, `texlive`, `texlive-full` | `MiKTeX.MiKTeX` |

## Autres langages

| Noms | Identifiant winget |
|---|---|
| `hurl` | `Orange-OpenSource.Hurl` |
| `mise` | `jdx.mise` |
| `elixir` | `Elixir.Elixir` |
| `erlang`, `erlang-otp` | `Erlang.ErlangOTP` |
| `dart`, `dart-sdk` | `Google.DartSDK` |
| `perl`, `strawberry-perl` | `StrawberryPerl.StrawberryPerl` |
| `r`, `r-base` | `RProject.R` |
| `julia`, `juliaup` | `Julialang.Juliaup` |
| `protoc`, `protobuf-compiler`, `protobuf` | `Google.Protobuf` |
| `sbt` | `sbt.sbt` |
| `swift`, `swiftlang` | `Swift.Toolchain` |
| `stack`, `haskell-stack` | `commercialhaskell.stack` |
