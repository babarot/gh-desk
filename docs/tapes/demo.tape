# The demo in the README: browse the pull requests that reach you, ask
# Claude about one, and merge the dependency updates together.
#
# Run with: mise run demo
# gh-desk runs for real against a made-up GitHub and Claude (internal/demo).

Output demo.mp4

# The colors of gh-desk's default theme
Set Theme { "name": "gh-desk", "black": "#1e1e2e", "red": "#f5a3b5", "green": "#b5f0b0", "yellow": "#ffe5a0", "blue": "#96bfff", "magenta": "#d4b0ff", "cyan": "#94e2d5", "white": "#d0d6e8", "brightBlack": "#706f87", "brightRed": "#f5a3b5", "brightGreen": "#b5f0b0", "brightYellow": "#ffe5a0", "brightBlue": "#96bfff", "brightMagenta": "#d4b0ff", "brightCyan": "#94e2d5", "brightWhite": "#e0e4f0", "background": "#1e1e2e", "foreground": "#e0e4f0", "selection": "#3b3b52", "cursor": "#e0e4f0" }

Set Shell "bash"
Set FontSize 16
Set Width 1400
Set Height 860
Set Padding 20
Set TypingSpeed 60ms

Hide
Type "export COLORTERM=truecolor PS1='$ '; gh() { /data/.gh-desk-demo; }; clear"
Enter
Sleep 500ms
Show

Sleep 500ms
Type "gh desk"
Sleep 400ms
Enter
Sleep 3.5s

# Look over the pull requests asked of you, and the diff of one
Type "j"
Sleep 700ms
Type "j"
Sleep 700ms
Type "k"
Sleep 400ms
Type "k"
Sleep 1.5s
Type "v"
Sleep 2s
Type "J"
Sleep 300ms
Type "J"
Sleep 300ms
Type "J"
Sleep 1.5s
Type "v"
Sleep 800ms

# Ask Claude about it
Type "i"
Sleep 2s
Type "Is it safe to retry PUT requests here?"
Sleep 500ms
Enter
Sleep 8.5s
Sleep 3s
Escape
Sleep 1s

# Select the dependency updates by a rule, and merge them together
Type "S"
Sleep 1.8s
Type "j"
Sleep 400ms
Type "j"
Sleep 1.8s
Enter
Sleep 1.5s
Type "M"
Sleep 4.5s
Sleep 3s
Type "y"
Sleep 9s

# The keys, and the version
Type "?"
Sleep 2.5s
Type "?"
Sleep 500ms
Type "q"
Sleep 1s
