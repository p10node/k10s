# k10s demo — keyboard-only, reproducible clip for docs and release notes.
#
#   vhs assets/demo.tape        (or `just demo`)
#
# Needs `k10s` on PATH (`just install`) and charmbracelet/vhs installed.
# `k10s demo` runs the offline demo backend, which is what every published
# frame should use — see docs/build-in-public.md. Plain `k10s` would show
# whatever cluster the recording machine can reach, or none at all.
#
# The mouse is k10s' differentiator and vhs cannot record a cursor: pair this
# with a real screen capture for the hero clip.

Output assets/demo.gif

Set Shell "bash"
Set FontSize 16
Set Width 1200
Set Height 700
Set Padding 20

Type "k10s demo"
Enter
Sleep 3s

# dismiss the first-run onboarding
Escape
Sleep 1s

# down to the CrashLoopBackOff pod, then follow its logs
Down
Down
Down
Sleep 1s
Type "l"
Sleep 3s
Escape
Sleep 1s

# search everything: kinds and objects in one box
Ctrl+P
Sleep 1s
Type "svc"
Sleep 2s
Enter
Sleep 2s

# theme picker — applies each theme live as you move
Type "/theme"
Enter
Sleep 1s
Down
Sleep 1s
Down
Sleep 1s
Down
Sleep 1s
Escape
Sleep 1s

# AI mode, with the cluster context already injected
Ctrl+A
Sleep 1s
Type "why is billing-worker crashing?"
Enter
Sleep 4s
