# Crit — multi-file code review

Output demo/code-review.gif

Require crit

Set Shell "zsh"
Set FontSize 16
Set Width 1400
Set Height 800
Set Theme "Catppuccin Mocha"
Set Padding 20
Set TypingSpeed 50ms
Set WindowBar Colorful

# Open multi-file code review
Type "crit review --code"
Sleep 500ms
Enter
Sleep 2s

# Scroll down to see some changes
Type@80ms "jjjjjjjj"
Sleep 800ms

# Jump to the next change
Type "n"
Sleep 800ms

# Add a comment on this change
Enter
Sleep 1s

# Type the comment
Type@40ms "This change looks good but needs a test."
Sleep 800ms

# Save the comment
Ctrl+S
Sleep 1.5s

# Switch to next file tab
Tab
Sleep 1.5s

# Scroll through the second file
Type@80ms "jjjjj"
Sleep 800ms

# Jump to next change in this file
Type "n"
Sleep 800ms

# Visual select a block
Type "v"
Sleep 400ms
Type@100ms "jjjj"
Sleep 800ms

# Add a comment on the selection
Enter
Sleep 1s

Type@40ms "Consider extracting this into a helper function."
Sleep 800ms

# Save the comment
Ctrl+S
Sleep 1.5s

# Quit
Type "q"
Sleep 2s
