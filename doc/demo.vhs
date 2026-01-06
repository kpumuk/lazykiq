Output doc/assets/demo.gif

Set Shell "bash"
Set Framerate 24
Set FontFamily "JetBrainsMono Nerd Font"
Set Theme "Catppuccin Macchiato"
# Doubled values for better quality
# See https://github.com/charmbracelet/vhs/issues/69#issuecomment-1295581303
Set FontSize 28
Set Width 2400
Set Height 1600
Set Padding 40

Hide
Type@1ms "go run ./cmd/lazykiq --redis redis://localhost:6379/1" Enter
Sleep 1s
Show

# Dashboard: stay 10s, but only record a few frames
Sleep 0.5s
Hide
Sleep 10s
Show
Sleep 0.5s
Hide
Sleep 10s
Show
Sleep 0.5s
Hide
Sleep 10s
Show
Sleep 2s
Screenshot "doc/assets/dashboard.png" Sleep 1s

# Busy
Type "2" Sleep 1.5s
Screenshot "doc/assets/busy.png" Sleep 1s

# Queues
Type "3" Sleep 1s
Down Sleep 0.5s Down Sleep 0.5s Enter Sleep 2s
Screenshot "doc/assets/job_details.png" Sleep 1s
Escape Sleep 0.5s
Screenshot "doc/assets/queue_details.png" Sleep 1s
Type "s" Sleep 1s
Screenshot "doc/assets/queues.png" Sleep 2s

# Retries
Type "4" Sleep 0.5s
Screenshot "doc/assets/retries.png" Sleep 1s
Type "]" Sleep 0.5s Type "]" Sleep 2s

# Scheduled
Type "5" Sleep 0.5s
Screenshot "doc/assets/scheduled.png" Sleep 1s
Type "/" Sleep 0.5s Type "Data" Sleep 0.5s Enter Sleep 1s

# Dead
Type "6" Sleep 1s
Screenshot "doc/assets/dead.png" Sleep 1s

# Errors
Type "7" Sleep 1s
Screenshot "doc/assets/errors_summary.png" Sleep 1s
Enter Sleep 2s
Screenshot "doc/assets/errors_details.png" Sleep 1s
Escape Sleep 0.5s

# Metrics
Type "8" Sleep 1s
Screenshot "doc/assets/metrics.png" Sleep 1s
Enter Sleep 2s
Screenshot "doc/assets/job_metrics.png" Sleep 1s

Type "q"