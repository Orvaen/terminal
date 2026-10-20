package apt

import (
	"sort"
	"strings"
)

// aliasEntry : des noms Linux (Debian/Ubuntu, Arch, Homebrew) et l'identifiant winget qu'ils désignent.
// Le premier nom est le nom principal, proposé par la complétion de « apt remove ».
type aliasEntry struct {
	Names []string
	ID    string
}

type aliasGroup struct {
	Title   string
	Entries []aliasEntry
}

// aliasGroups : la table des noms connus, par thème. Les identifiants ont été vérifiés dans le dépôt
// des manifestes winget (microsoft/winget-pkgs). Un nom absent est cherché dans winget (resolve.go).
// La liste lisible, docs/paquets.md, est générée depuis cette table (voir paquets_doc_test.go).
//
// « docker », « terminal » (mise à jour), Composer, Maven, Gradle, Poetry et build-essential
// ne passent pas par cette table : voir isDocker, isSelf et tools.go.
var aliasGroups = []aliasGroup{
	{"Éditeurs et IDE", []aliasEntry{
		{[]string{"code", "vscode", "visual-studio-code", "visual-studio-code-bin"}, "Microsoft.VisualStudioCode"},
		{[]string{"code-insiders", "visual-studio-code-insiders"}, "Microsoft.VisualStudioCode.Insiders"},
		{[]string{"neovim", "nvim"}, "Neovim.Neovim"},
		{[]string{"vim", "gvim", "vim-gtk3"}, "vim.vim"},
		{[]string{"nano"}, "GNU.Nano"},
		{[]string{"emacs"}, "GNU.Emacs"},
		{[]string{"helix", "hx"}, "Helix.Helix"},
		{[]string{"sublime-text", "sublime-text-4", "subl"}, "SublimeHQ.SublimeText.4"},
		{[]string{"notepad++", "notepadplusplus"}, "Notepad++.Notepad++"},
		{[]string{"android-studio"}, "Google.AndroidStudio"},
		{[]string{"intellij-idea-community", "intellij-idea-community-edition", "idea"}, "JetBrains.IntelliJIDEA.Community"},
		{[]string{"pycharm-community", "pycharm-community-edition"}, "JetBrains.PyCharm.Community"},
		{[]string{"obsidian"}, "Obsidian.Obsidian"},
	}},
	{"Langages et outils de développement", []aliasEntry{
		{[]string{"git", "git-all"}, "Git.Git"},
		{[]string{"gh", "github-cli"}, "GitHub.cli"},
		{[]string{"glab"}, "GLab.GLab"},
		{[]string{"github-desktop"}, "GitHub.GitHubDesktop"},
		{[]string{"lazygit"}, "JesseDuffield.lazygit"},
		{[]string{"git-delta", "delta"}, "dandavison.delta"},
		{[]string{"node", "nodejs", "npm", "nodejs-lts"}, "OpenJS.NodeJS.LTS"},
		{[]string{"node-current", "nodejs-current"}, "OpenJS.NodeJS"},
		{[]string{"nvm", "nvm-windows"}, "CoreyButler.NVMforWindows"},
		{[]string{"yarn"}, "Yarn.Yarn"},
		{[]string{"pnpm"}, "pnpm.pnpm"},
		{[]string{"bun"}, "Oven-sh.Bun"},
		{[]string{"deno"}, "DenoLand.Deno"},
		{[]string{"python", "python3", "python-is-python3", "python3-pip", "pip", "python3-full"}, "Python.Python.3.14"},
		{[]string{"py", "python-launcher"}, "Python.Launcher"},
		{[]string{"uv"}, "astral-sh.uv"},
		{[]string{"ruff"}, "astral-sh.ruff"},
		{[]string{"miniconda", "miniconda3", "conda"}, "Anaconda.Miniconda3"},
		{[]string{"go", "golang", "golang-go"}, "GoLang.Go"},
		{[]string{"rust", "rustup", "rustc", "cargo"}, "Rustlang.Rustup"},
		{[]string{"zig"}, "zig.zig"},
		{[]string{"llvm", "clang", "clang-format", "lld"}, "LLVM.LLVM"},
		{[]string{"gcc", "g++", "gdb", "mingw", "mingw-w64", "gcc-mingw-w64", "winlibs"}, "BrechtSanders.WinLibs.POSIX.UCRT"},
		{[]string{"msys2"}, "MSYS2.MSYS2"},
		{[]string{"make"}, "ezwinports.make"},
		{[]string{"cmake"}, "Kitware.CMake"},
		{[]string{"java", "jdk", "default-jdk", "default-jre", "openjdk", "openjdk-25-jdk", "temurin", "jdk25"}, "EclipseAdoptium.Temurin.25.JDK"},
		{[]string{"openjdk-21-jdk", "openjdk-21-jre", "jdk21", "openjdk21", "jdk-openjdk21"}, "EclipseAdoptium.Temurin.21.JDK"},
		{[]string{"openjdk-17-jdk", "openjdk-17-jre", "jdk17", "openjdk17"}, "EclipseAdoptium.Temurin.17.JDK"},
		{[]string{"dotnet", "dotnet-sdk", "dotnet-sdk-10.0"}, "Microsoft.DotNet.SDK.10"},
		{[]string{"dotnet-sdk-9.0"}, "Microsoft.DotNet.SDK.9"},
		{[]string{"dotnet-sdk-8.0"}, "Microsoft.DotNet.SDK.8"},
		{[]string{"php", "php8.5", "php-cli"}, "PHP.PHP.8.5"},
		{[]string{"php8.4"}, "PHP.PHP.8.4"},
		{[]string{"ruby", "ruby-full", "rubygems"}, "RubyInstallerTeam.RubyWithDevKit.3.4"},
		{[]string{"postgresql", "postgres", "psql", "postgresql-18"}, "PostgreSQL.PostgreSQL.18"},
		{[]string{"postgresql-17"}, "PostgreSQL.PostgreSQL.17"},
		{[]string{"mysql", "mysql-server"}, "Oracle.MySQL"},
		{[]string{"mongodb", "mongod", "mongodb-server"}, "MongoDB.Server"},
		{[]string{"redis", "redis-server"}, "Redis.Redis"},
		{[]string{"sqlite", "sqlite3"}, "SQLite.SQLite"},
		{[]string{"postman"}, "Postman.Postman"},
		{[]string{"graphviz", "dot"}, "Graphviz.Graphviz"},
		{[]string{"pandoc"}, "JohnMacFarlane.Pandoc"},
		{[]string{"hugo", "hugo-extended"}, "Hugo.Hugo.Extended"},
		{[]string{"caddy"}, "CaddyServer.Caddy"},
		{[]string{"ollama"}, "Ollama.Ollama"},
	}},
	{"Conteneurs et cloud", []aliasEntry{
		{[]string{"docker-desktop"}, "Docker.DockerDesktop"},
		{[]string{"podman"}, "RedHat.Podman"},
		{[]string{"podman-desktop"}, "RedHat.Podman-Desktop"},
		{[]string{"rancher-desktop"}, "SUSE.RancherDesktop"},
		{[]string{"kubectl", "kubernetes-cli", "kubernetes-client"}, "Kubernetes.kubectl"},
		{[]string{"minikube"}, "Kubernetes.minikube"},
		{[]string{"kind"}, "Kubernetes.kind"},
		{[]string{"helm"}, "Helm.Helm"},
		{[]string{"k3d"}, "k3d.k3d"},
		{[]string{"terraform"}, "Hashicorp.Terraform"},
		{[]string{"awscli", "aws-cli", "aws"}, "Amazon.AWSCLI"},
		{[]string{"azure-cli", "az"}, "Microsoft.AzureCLI"},
		{[]string{"wsl"}, "Microsoft.WSL"},
	}},
	{"Ligne de commande", []aliasEntry{
		{[]string{"coreutils", "uutils-coreutils"}, "Microsoft.Coreutils"},
		{[]string{"curl"}, "cURL.cURL"},
		{[]string{"wget"}, "JernejSimoncic.Wget"},
		{[]string{"jq"}, "jqlang.jq"},
		{[]string{"ripgrep", "rg"}, "BurntSushi.ripgrep.MSVC"},
		{[]string{"fd", "fd-find"}, "sharkdp.fd"},
		{[]string{"bat", "batcat"}, "sharkdp.bat"},
		{[]string{"fzf"}, "junegunn.fzf"},
		{[]string{"eza", "exa"}, "eza-community.eza"},
		{[]string{"zoxide"}, "ajeetdsouza.zoxide"},
		{[]string{"starship"}, "Starship.Starship"},
		{[]string{"btop", "htop"}, "aristocratos.btop4win"},
		{[]string{"7zip", "p7zip", "p7zip-full", "7z", "7-zip"}, "7zip.7zip"},
		{[]string{"ffmpeg"}, "Gyan.FFmpeg"},
		{[]string{"imagemagick", "magick"}, "ImageMagick.ImageMagick"},
		{[]string{"pwsh", "powershell"}, "Microsoft.PowerShell"},
		{[]string{"rclone"}, "Rclone.Rclone"},
		{[]string{"nmap"}, "Insecure.Nmap"},
		{[]string{"wezterm"}, "wez.wezterm"},
		{[]string{"alacritty"}, "Alacritty.Alacritty"},
		{[]string{"terminal", "windows-terminal"}, "Microsoft.WindowsTerminal"},
		{[]string{"powertoys"}, "Microsoft.PowerToys"},
	}},
	{"Applications", []aliasEntry{
		{[]string{"chrome", "google-chrome", "google-chrome-stable"}, "Google.Chrome"},
		{[]string{"chromium", "chromium-browser", "ungoogled-chromium"}, "Hibbiki.Chromium"},
		{[]string{"firefox"}, "Mozilla.Firefox"},
		{[]string{"sccache"}, "Mozilla.sccache"},
		{[]string{"firefox-esr"}, "Mozilla.Firefox.ESR"},
		{[]string{"brave", "brave-browser"}, "Brave.Brave"},
		{[]string{"vivaldi", "vivaldi-stable"}, "Vivaldi.Vivaldi"},
		{[]string{"opera"}, "Opera.Opera"},
		{[]string{"edge", "microsoft-edge", "microsoft-edge-stable"}, "Microsoft.Edge"},
		{[]string{"thunderbird"}, "Mozilla.Thunderbird"},
		{[]string{"discord"}, "Discord.Discord"},
		{[]string{"slack", "slack-desktop"}, "SlackTechnologies.Slack"},
		{[]string{"telegram", "telegram-desktop"}, "Telegram.TelegramDesktop"},
		{[]string{"zoom"}, "Zoom.Zoom"},
		{[]string{"teams", "microsoft-teams"}, "Microsoft.Teams"},
		{[]string{"spotify", "spotify-client"}, "Spotify.Spotify"},
		{[]string{"vlc"}, "VideoLAN.VLC"},
		{[]string{"mpv"}, "mpv.net"},
		{[]string{"obs", "obs-studio"}, "OBSProject.OBSStudio"},
		{[]string{"gimp"}, "GIMP.GIMP.3"},
		{[]string{"inkscape"}, "Inkscape.Inkscape"},
		{[]string{"blender"}, "BlenderFoundation.Blender"},
		{[]string{"audacity"}, "Audacity.Audacity"},
		{[]string{"handbrake"}, "HandBrake.HandBrake"},
		{[]string{"libreoffice", "libreoffice-fresh", "libreoffice-still"}, "TheDocumentFoundation.LibreOffice"},
		{[]string{"keepassxc"}, "KeePassXCTeam.KeePassXC"},
		{[]string{"bitwarden"}, "Bitwarden.Bitwarden"},
		{[]string{"qbittorrent"}, "qBittorrent.qBittorrent"},
		{[]string{"steam", "steam-installer"}, "Valve.Steam"},
		{[]string{"wireshark", "wireshark-qt"}, "WiresharkFoundation.Wireshark"},
		{[]string{"putty"}, "PuTTY.PuTTY"},
		{[]string{"winscp"}, "WinSCP.WinSCP"},
		{[]string{"wireguard", "wireguard-tools"}, "WireGuard.WireGuard"},
		{[]string{"openvpn"}, "OpenVPNTechnologies.OpenVPN"},
		{[]string{"tailscale"}, "Tailscale.Tailscale"},
		{[]string{"syncthing"}, "Syncthing.Syncthing"},
		{[]string{"miktex", "texlive", "texlive-full"}, "MiKTeX.MiKTeX"},
	}},
	{"Autres langages", []aliasEntry{
		{[]string{"hurl"}, "Orange-OpenSource.Hurl"},
		{[]string{"mise"}, "jdx.mise"},
		{[]string{"elixir"}, "Elixir.Elixir"},
		{[]string{"erlang", "erlang-otp"}, "Erlang.ErlangOTP"},
		{[]string{"dart", "dart-sdk"}, "Google.DartSDK"},
		{[]string{"perl", "strawberry-perl"}, "StrawberryPerl.StrawberryPerl"},
		{[]string{"r", "r-base"}, "RProject.R"},
		{[]string{"julia", "juliaup"}, "Julialang.Juliaup"},
		{[]string{"protoc", "protobuf-compiler", "protobuf"}, "Google.Protobuf"},
		{[]string{"sbt"}, "sbt.sbt"},
		{[]string{"swift", "swiftlang"}, "Swift.Toolchain"},
		{[]string{"stack", "haskell-stack"}, "commercialhaskell.stack"},
	}},
}

// aliases : nom → identifiant winget ; primaryName : identifiant → nom principal.
var aliases, primaryName = buildAliases()

func buildAliases() (map[string]string, map[string]string) {
	names, primary := map[string]string{}, map[string]string{}
	for _, g := range aliasGroups {
		for _, e := range g.Entries {
			for _, n := range e.Names {
				names[n] = e.ID
			}
			if _, ok := primary[e.ID]; !ok {
				primary[e.ID] = e.Names[0]
			}
		}
	}
	return names, primary
}

// Resolve renvoie l'identifiant winget d'un nom connu (tes noms d'abord, puis la table),
// et true s'il s'agit d'un identifiant exact.
func Resolve(name string) (string, bool) {
	if id, ok := userAlias(name); ok {
		return id, true
	}
	if id, ok := aliases[strings.ToLower(name)]; ok {
		return id, true
	}
	return name, false
}

// AliasNames renvoie la liste triée des noms connus.
func AliasNames() []string {
	names := make([]string, 0, len(aliases))
	for n := range aliases {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
