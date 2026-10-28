# Copy outside the repository and adapt paths, readiness, and actions to the PR.
# Build the selected revision to this launch path before running VHS.
Output "/tmp/lazykiq-pr-evidence/after.mp4"

# Match doc/demo.vhs; refresh these settings if its style changes.
Set Shell "bash"
Set Framerate 24
Set FontFamily "JetBrainsMono Nerd Font"
Set Theme "Catppuccin Macchiato"
Set FontSize 28
Set Width 2400
Set Height 1600
Set Padding 40

Hide
# VHS may set NO_COLOR=1; preserve normal application colors.
Type@1ms "env -u NO_COLOR COLORTERM=truecolor /tmp/lazykiq-pr-evidence/lazykiq --redis redis://localhost:6379/1" Enter
Wait+Screen /Processed/
Show
Sleep 1s

# Replace with the affected view and interaction; keep transitions visible.
Type "3"
Sleep 1s
Screenshot "/tmp/lazykiq-pr-evidence/after.png"
Sleep 2s

Hide
Type "q"
