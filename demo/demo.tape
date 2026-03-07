# Crit — terminal review tool for markdown documents

Output demo/demo.gif

Require crit

Set Shell "zsh"
Set FontSize 16
Set Width 1400
Set Height 800
Set Theme "Catppuccin Mocha"
Set Padding 20
Set TypingSpeed 50ms
Set WindowBar Colorful

# Open the review TUI
Type "crit demo/plan.md"
Sleep 500ms
Enter
Sleep 1.5s

# Scroll down to the json block (line 12)
Type@80ms "jjjjjjjjjjj"
Sleep 800ms

# Enter visual select mode on the ```json line
Type "v"
Sleep 400ms

# Select down to line 20 (the closing ```) — 8 more lines
Type@100ms "jjjjjjjj"
Sleep 800ms

# Press enter to open the comment modal
Enter
Sleep 1s

# Type the comment
Type@40ms "Add a status field — consumers need to know if the deploy succeeded or failed."
Sleep 800ms

# Save the comment
Ctrl+S
Sleep 2s
