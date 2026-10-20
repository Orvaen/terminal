"""Génère l'identité visuelle de Terminal (un logiciel Arkerion) et les visuels de la documentation.

    python docs/brand/build.py

Les polices Arkerion Sans et Arkerion Mono sont lues dans le dépôt Arkerion-Docs
(variable d'environnement ARKERION_FONTS, ou dossier frère ../arkerion-docs/src/fonts).
Tout le texte est vectorisé : les fichiers produits n'ont besoin d'aucune police.

Règles reprises de l'identité Arkerion (Arkerion-Docs, pages Logo et Nebula) :
grille de 64 unités, modules aux coins arrondis (8 dehors, 6 dedans),
arrondi signature de 18 en haut à droite du module principal, logo en noir
#0D0E10 ou en blanc uniquement, nom composé en Arkerion Sans resserré (-25).
L'Améthyste n'est utilisée que pour l'interface (documentation, aperçus, pictogrammes).
"""

import os
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from arkbrand import (  # noqa: E402
    Font,
    Scene,
    compose,
    ops_bounds,
    ops_from_d,
    rect_d,
    rotate_about,
    round_poly,
    scale,
    transform_ops,
    translate,
)

OUT = Path(__file__).parent
ROOT = OUT.parent.parent


def find_fonts():
    candidates = [
        os.environ.get("ARKERION_FONTS"),
        ROOT.parent / "arkerion-docs" / "src" / "fonts",
        ROOT.parent / "Arkerion-Docs" / "src" / "fonts",
        Path("/home/claude/orvaen/arkerion-docs/src/fonts"),
    ]
    for c in candidates:
        if c and (Path(c) / "ArkerionSans-Regular.ttf").exists():
            return Path(c)
    sys.exit("Polices Arkerion introuvables : définis ARKERION_FONTS (dossier src/fonts d'Arkerion-Docs).")


FONTS = find_fonts()
SANS = Font(FONTS / "ArkerionSans-Regular.ttf")
MONO = Font(FONTS / "ArkerionMono-Regular.ttf")
TRACK = -25  # composition resserrée du nom, comme les logos Arkerion et Nebula

# Logo : noir et blanc uniquement
INK = "#0D0E10"
WHITE = "#FFFFFF"

# Interface : neutres doux + Améthyste Arkerion
THEMES = {
    "clair": dict(
        bg="#F6F5F8", surface="#FFFFFF", bar="#FBFAFC", hair="#E6E3EC", logo=INK,
        text="#2B2A30", soft="#4A4752", mute="#76727F", accent="#6E569F", accent_soft="#EEEAF5",
    ),
    "sombre": dict(
        bg="#141317", surface="#1B1A1F", bar="#1F1E24", hair="#2E2C35", logo=WHITE,
        text="#ECEAF0", soft="#C9C5D1", mute="#9A96A4", accent="#B4A4D6", accent_soft="#29243A",
    ),
}

# ---------------------------------------------------------------------------
# Le symbole « Invite »
#
# Un module principal, l'écran, avec l'arrondi signature d'Arkerion en haut à droite.
# L'invite de commande y est creusée en modules : un chevron de trois carrés en
# diagonale et le curseur. Même grille, mêmes arrondis que Structure et Amas.

SCREEN = "M26 18H64A18 18 0 0 1 82 36V74A8 8 0 0 1 74 82H26A8 8 0 0 1 18 74V26A8 8 0 0 1 26 18Z"
MODULE, MODULE_R = 10, 3
PROMPT = (  # (x, y, largeur, hauteur) des modules de l'invite
    (29, 35, MODULE, MODULE),
    (39, 45, MODULE, MODULE),
    (29, 55, MODULE, MODULE),
    (53, 55, 16, MODULE),  # curseur
)


def symbol_ops():
    d = SCREEN + "".join(rect_d(x, y, w, h, MODULE_R) for x, y, w, h in PROMPT)
    return ops_from_d(d)


def prompt_ops():
    """L'invite seule (sans écran), pour les pictogrammes."""
    return ops_from_d("".join(rect_d(x, y, w, h, MODULE_R) for x, y, w, h in PROMPT))


def place(ops, x, y, size):
    """Place un dessin de la grille 18..82 dans un carré (x, y, size)."""
    return transform_ops(ops, compose(translate(x, y), compose(scale(size / 64), translate(-18, -18))))


# ---------------------------------------------------------------------------
# Fichiers du logo


def write(name, content):
    (OUT / name).write_text(content, encoding="utf-8")
    print("  ", name)


def name_ops(size=1000, x=0, y=0):
    return SANS.layout("Terminal", size, x, y, tracking=TRACK)


def build_logo():
    print("Logo")
    for variant, color in (("noir", INK), ("blanc", WHITE)):
        # symbole seul (grille 18..82)
        s = Scene(64, 64, title="Terminal, symbole")
        s.add(symbol_ops(), color, rule="evenodd")
        write(f"terminal-symbole-{variant}.svg", s.svg(display_width=512, viewbox=(18, 18, 64, 64)))

        # logo complet : même construction que Nebula (symbole de 1000 unités, nom à 1250)
        word, _ = name_ops()
        wb = ops_bounds(word)
        logo = Scene(0, 0, title="Terminal")
        logo.add(place(symbol_ops(), 0, -850, 1000), color, rule="evenodd")
        logo.add(transform_ops(word, translate(1250, 0)), color)
        width = 1250 + wb[2]
        write(f"terminal-logo-{variant}.svg", logo.svg(display_width=width / 5, viewbox=(0, -850, width, 1000)))

        # nom seul
        nom = Scene(0, 0, title="Terminal")
        nom.add(word, color)
        pad = 20
        vb = (wb[0] - pad, wb[1] - pad, wb[2] - wb[0] + 2 * pad, wb[3] - wb[1] + 2 * pad)
        write(f"terminal-nom-{variant}.svg", nom.svg(display_width=vb[2] / 4, viewbox=vb))

    # avatar carré (fond plein), comme nebula-avatar
    av = Scene(100, 100, background=WHITE, title="Terminal")
    av.add(place(symbol_ops(), 50 - 32 * 0.82, 50 - 32 * 0.82, 64 * 0.82), INK, rule="evenodd")
    write("terminal-avatar-noir.svg", av.svg(display_width=1024))
    av.png(OUT / "terminal-avatar-noir.png", 1024)
    print("   terminal-avatar-noir.png")


def build_icon():
    """Icône Windows : symbole noir bordé de blanc, lisible sur barre des tâches claire ou sombre."""
    print("Icône")
    from PIL import Image

    sizes = [256, 64, 48, 40, 32, 24, 20, 16]
    images = []
    for px in sizes:
        rim = 2.2 if px <= 32 else 1.4  # liseré blanc, en unités de la grille
        sc = Scene(64 + 2 * rim, 64 + 2 * rim)
        outer = transform_ops(
            ops_from_d("M26 18H64A18 18 0 0 1 82 36V74A8 8 0 0 1 74 82H26A8 8 0 0 1 18 74V26A8 8 0 0 1 26 18Z"),
            compose(translate(32 + rim, 32 + rim), compose(scale((64 + 2 * rim) / 64), translate(-50, -50))),
        )
        sc.add(outer, WHITE)
        sc.add(transform_ops(symbol_ops(), translate(rim - 18, rim - 18)), INK, rule="evenodd")
        tmp = OUT / f".icon-{px}.png"
        images.append(sc.png(tmp, px, supersample=8))
        tmp.unlink()
    images[0].save(OUT / "terminal.ico", sizes=[(s, s) for s in sizes], append_images=images[1:])
    print("   terminal.ico")


# ---------------------------------------------------------------------------
# Visuels de la documentation


def module_dot(scene, x, y, size, color):
    scene.add_rect(x, y, size, size, size * 0.3, color)


def text(scene, font, s, size, x, y, color, tracking=0, anchor="start"):
    ops, w = font.layout(s, size, 0, 0, tracking=tracking)
    if anchor == "middle":
        x -= w / 2
    elif anchor == "end":
        x -= w
    scene.add(transform_ops(ops, translate(x, y)), color)
    return w


def lockup(scene, x, y, height, color, center=False):
    """Logo complet (symbole + nom) de hauteur de symbole `height` ; renvoie la largeur."""
    k = height / 1000
    word, _ = name_ops()
    wb = ops_bounds(word)
    width = (1250 + wb[2]) * k
    if center:
        x -= width / 2
    scene.add(place(symbol_ops(), x, y, height), color, rule="evenodd")
    scene.add(transform_ops(word, compose(translate(x + 1250 * k, y + 850 * k), scale(k))), color)
    return width


def build_banner():
    print("Bannière")
    W, H = 1600, 560
    for theme, c in THEMES.items():
        s = Scene(W, H, title="Terminal, un logiciel Arkerion")
        s.add_rect(0, 0, W, H, 28, c["bg"])
        lockup(s, W / 2, 128, 150, c["logo"], center=True)
        text(s, SANS, "Le terminal façon Linux pour Windows.", 46, W / 2, 380, c["text"], TRACK, anchor="middle")
        # apt  sudo  docker, séparés par des modules Améthyste
        words = ["apt", "sudo", "docker", "sans installateurs"]
        size, gap = 32, 44
        widths = [MONO.width(w_, size) if i < 3 else SANS.width(w_, size, TRACK) for i, w_ in enumerate(words)]
        total = sum(widths) + gap * (len(words) - 1)
        x = W / 2 - total / 2
        for i, (w_, ww) in enumerate(zip(words, widths)):
            font = MONO if i < 3 else SANS
            text(s, font, w_, size, x, 448, c["accent"] if i < 3 else c["mute"], 0 if i < 3 else TRACK)
            x += ww
            if i < len(words) - 1:
                module_dot(s, x + gap / 2 - 5, 448 - 15, 10, c["accent"] if i < 2 else c["mute"])
                x += gap
        text(s, SANS, "Un logiciel Arkerion", 26, 64, H - 48, c["mute"], TRACK)
        write(f"banniere-{theme}.svg", s.svg(display_width=W / 2 * 1.1))
        if theme == "sombre":
            s.png(OUT / ".apercu-banniere.png", 1100)


SESSION = [
    ("prompt", "~/projets/site"),
    ("cmd", "sudo apt update && apt install git node"),
    ("step", "Mise à jour des sources"),
    ("step", "Installation de git"),
    ("step", "Installation de node"),
    ("blank", ""),
    ("prompt", "~/projets/site"),
    ("cmd", "docker run -d -p 8080:80 -v .:/usr/share/nginx/html nginx"),
    ("out", "Démarrage du moteur Docker..."),
    ("out", "3f9c2a7e1b04"),
    ("blank", ""),
    ("prompt", "~/projets/site"),
    ("cursor", ""),
]


def build_preview():
    print("Aperçu")
    W, pad, bar = 1600, 56, 72
    size, lh = 28, 46
    H = bar + pad + lh * len(SESSION) + pad - 10
    for theme, c in THEMES.items():
        s = Scene(W, H, title="Aperçu de Terminal")
        # fenêtre
        s.add_rect(0, 0, W, H, 24, c["hair"])
        s.add_rect(2, 2, W - 4, H - 4, 22, c["surface"])
        s.add_d(f"M24 2H{W - 24}A22 22 0 0 1 {W - 2} 24V{bar}H2V24A22 22 0 0 1 24 2Z", c["bar"])
        s.add_rect(2, bar, W - 4, 2, 0, c["hair"])
        # barre de titre : symbole + nom, commandes de fenêtre Windows
        s.add(place(symbol_ops(), 32, bar / 2 - 13, 26), c["logo"], rule="evenodd")
        text(s, SANS, "Terminal", 24, 70, bar / 2 + 8.5, c["soft"], TRACK)
        cx, cy = W - 150, bar / 2
        s.add_rect(cx, cy, 18, 2, 1, c["mute"])  # réduire
        mx = cx + 52
        s.add_d(rect_d(mx, cy - 9, 18, 18, 3) + rect_d(mx + 2, cy - 7, 14, 14, 1.5), c["mute"])  # agrandir
        xx = mx + 52 + 9
        for ang in (45, -45):  # fermer
            s.add(ops_from_d(rect_d(xx - 12, cy - 1, 24, 2, 1), rotate_about(ang, xx, cy)), c["mute"])
        # session
        y = bar + pad + 20
        x0 = pad
        cw = MONO.width("a", size)
        for kind, val in SESSION:
            if kind == "prompt":
                x = x0
                x += text(s, MONO, "dev@PC", size, x, y, c["accent"]) + cw
                x += text(s, MONO, "WIN", size, x, y, c["mute"]) + cw
                text(s, MONO, val, size, x, y, c["soft"])
            elif kind == "cmd":
                text(s, MONO, "$", size, x0, y, c["accent"])
                text(s, MONO, val, size, x0 + 2 * cw, y, c["text"])
            elif kind == "step":
                text(s, MONO, "==>", size, x0, y, c["accent"])
                text(s, MONO, val, size, x0 + 4 * cw, y, c["text"])
            elif kind == "out":
                text(s, MONO, val, size, x0, y, c["mute"])
            elif kind == "cursor":
                text(s, MONO, "$", size, x0, y, c["accent"])
                s.add_rect(x0 + 2 * cw, y - size * 0.78, cw, size * 0.98, 2, c["accent"])
            y += lh
        write(f"apercu-{theme}.svg", s.svg(display_width=W / 2 * 1.1))
        if theme == "clair":
            s.png(OUT / ".apercu-session.png", 1100)


# Pictogrammes des fonctionnalités, dans la même grammaire (grille 64, modules arrondis)

def icon_paquets():
    # un colis : couvercle avec l'arrondi signature, corps, poignée en creux
    lid = round_poly([(18, 18), (82, 18), (82, 32), (18, 32)], [8, 12, 4, 4])
    body = rect_d(22, 38, 56, 44, 8) + rect_d(40, 46, 20, 8, 4)
    return ops_from_d(lid + body)


def icon_admin():
    # un cadenas : anse en arc + corps avec trou de serrure
    shackle = "M32 42V36A18 18 0 0 1 68 36V42H60V36A10 10 0 0 0 40 36V42Z"
    body = rect_d(20, 42, 60, 40, 8) + rect_d(46, 54, 8, 16, 4)
    return ops_from_d(shackle + body)


def icon_docker():
    # conteneurs empilés : trois modules, puis deux, puis un
    d = "".join(rect_d(x, y, 18, 18, 6) for x, y in ((18, 64), (41, 64), (64, 64), (29.5, 41), (52.5, 41), (41, 18)))
    return ops_from_d(d)


def icon_commandes():
    return prompt_ops()


ICONS = {
    "commandes": icon_commandes,
    "paquets": icon_paquets,
    "admin": icon_admin,
    "docker": icon_docker,
}


def build_icons():
    print("Pictogrammes")
    for name, fn in ICONS.items():
        ops = fn()
        if name == "commandes":  # l'invite seule est petite : on la recadre sur la grille
            b = ops_bounds(ops)
            k = 64 / max(b[2] - b[0], b[3] - b[1]) * 0.9
            ops = transform_ops(ops, compose(translate(50, 50), compose(scale(k), translate(-(b[0] + b[2]) / 2, -(b[1] + b[3]) / 2))))
        for theme, c in THEMES.items():
            s = Scene(64, 64)
            s.add(ops, c["accent"], rule="evenodd")
            write(f"icone-{name}-{theme}.svg", s.svg(display_width=56, viewbox=(14, 14, 72, 72)))


def build_docker_diagram():
    """Schéma : comment une commande docker atteint le moteur dans WSL2."""
    print("Schéma Docker")
    W, H = 1600, 520
    for theme, c in THEMES.items():
        s = Scene(W, H, title="Fonctionnement du Docker léger")
        s.add_rect(0, 0, W, H, 28, c["bg"])
        # deux zones : Windows et la machine Linux légère
        s.add_rect(48, 96, 640, 376, 20, c["surface"])
        s.add_rect(760, 96, 792, 376, 20, c["accent_soft"])
        text(s, SANS, "Windows", 26, 76, 74, c["mute"], TRACK)
        text(s, SANS, "WSL2, machine Alpine Linux (quelques Mo)", 26, 788, 74, c["accent"], TRACK)

        def box(x, y, w, h, title, sub, fill, title_color):
            s.add_rect(x, y, w, h, 14, fill)
            text(s, MONO, title, 28, x + 28, y + 50, title_color)
            text(s, SANS, sub, 22, x + 28, y + 86, c["mute"], TRACK)

        box(80, 132, 576, 112, "docker run ...", "Orvaen Terminal, PowerShell, VS Code", c["bg"], c["text"])
        box(80, 328, 576, 112, "docker.exe", "convertit C:\\ en /mnt/c, démarre le moteur", c["bg"], c["text"])
        box(792, 132, 728, 112, "Docker Engine", "démarré à la demande, arrêt après 10 min", c["surface"], c["accent"])
        # conteneurs : modules
        for i in range(3):
            s.add_rect(792 + i * 250, 328, 228, 112, 14, c["surface"])
            module_dot(s, 792 + i * 250 + 28, 328 + 34, 44, c["accent"])
            text(s, SANS, "conteneur", 24, 792 + i * 250 + 92, 328 + 66, c["soft"], TRACK)

        def arrow(x1, y1, x2, y2):
            # trait + pointe faits de modules arrondis
            if x1 == x2:
                s.add_rect(x1 - 2, min(y1, y2), 4, abs(y2 - y1) - 10, 2, c["mute"])
                s.add_d(f"M{x2 - 11} {y2 - 14}L{x2} {y2}L{x2 + 11} {y2 - 14}Z", c["mute"])
            else:
                s.add_rect(min(x1, x2), y1 - 2, abs(x2 - x1) - 10, 4, 2, c["mute"])
                s.add_d(f"M{x2 - 14} {y2 - 11}L{x2} {y2}L{x2 - 14} {y2 + 11}Z", c["mute"])

        arrow(368, 252, 368, 320)
        # docker.exe -> wsl.exe -> moteur : le trait passe entre les deux zones
        s.add_rect(656, 382, 70, 4, 2, c["mute"])
        s.add_rect(722, 186, 4, 200, 2, c["mute"])
        s.add_rect(722, 186, 58, 4, 2, c["mute"])
        s.add_d("M792 188L778 177L778 199Z", c["mute"])
        arrow(1156, 252, 1156, 320)
        text(s, SANS, "wsl.exe", 20, 690, 172, c["mute"], TRACK, anchor="middle")
        write(f"schema-docker-{theme}.svg", s.svg(display_width=W / 2 * 1.1))
        if theme == "clair":
            s.png(OUT / ".apercu-schema.png", 1100)


def build_share():
    print("Carte de partage")
    W, H = 1280, 640
    s = Scene(W, H, background=INK, title="Terminal, un logiciel Arkerion")
    lockup(s, W / 2, 251, 138, WHITE, center=True)
    text(s, SANS, "Un logiciel Arkerion", 26, 80, 567, "#8A8794", TRACK)
    s.png(OUT / "terminal-partage.png", W)
    print("   terminal-partage.png")


def build_sheet():
    """Planche des déclinaisons, pour la page Identité."""
    W, H = 1600, 720
    s = Scene(W, H)
    s.add_rect(0, 0, W / 2, H, 0, WHITE)
    s.add_rect(W / 2, 0, W / 2, H, 0, INK)
    for i, color in enumerate((INK, WHITE)):
        x0 = i * W / 2
        lockup(s, x0 + W / 4, 110, 120, color, center=True)
        s.add(place(symbol_ops(), x0 + 120, 330, 220), color, rule="evenodd")
        for j, px in enumerate((96, 48, 32, 16)):
            s.add(place(symbol_ops(), x0 + 400 + j * 100 + (96 - px) / 2, 330 + (220 - px) / 2, px), color, rule="evenodd")
        word, _ = name_ops(64)
        s.add(transform_ops(word, translate(x0 + 120, 650)), color)
    write("planche.svg", s.svg(display_width=1100))
    s.png(OUT / ".apercu-planche.png", 1100)


if __name__ == "__main__":
    build_logo()
    build_icon()
    build_banner()
    build_preview()
    build_icons()
    build_docker_diagram()
    build_share()
    build_sheet()
    print("Terminé.")
