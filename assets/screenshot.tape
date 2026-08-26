# k10s hero screenshot — a real terminal capture of the running TUI, used at
# the top of the README.
#
#   vhs assets/screenshot.tape        (or `just screenshot`)
#
# Needs charmbracelet/vhs, a JetBrainsMono Nerd Font install (the header's ⎈
# and the CPU/MEM bars need the glyph coverage) and a local `./k10s` build.
#
# KUBECONFIG points nowhere on purpose: every published frame runs on the
# offline demo backend, so no real cluster ever leaks into the docs. The
# throwaway GIF goes to /tmp — the PNG is the only artifact we keep.

Output "/tmp/k10s-screenshot-scratch.gif"

Set Shell "bash"
Set FontFamily "JetBrainsMono Nerd Font"
Set FontSize 15
Set Width 1600
Set Height 780
Set Padding 20
Set WindowBar Colorful
Set BorderRadius 10
Set Margin 20
Set MarginFill "#16161e"
Set Theme { "background": "#1a1b26", "foreground": "#c0caf5", "cursor": "#c0caf5" }

Env KUBECONFIG "/nonexistent/kubeconfig"
Env K10S_CONFIG "/tmp/k10s-screenshot-config.yaml"

# The launch itself is not interesting — hide it, and let the alt screen hide
# the shell prompt behind it.
Hide
Type "./k10s"
Enter
Sleep 6s

# dismiss the first-run onboarding
Escape
Sleep 2s
Show
Sleep 2s

Screenshot assets/screenshot.png
