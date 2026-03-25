Output docs/demo/status.gif

Require bash
Require git
Require gh

Set Shell "bash"
Set FontFamily "Monaco"
Set FontSize 18
Set Width 1380
Set Height 920
Set Padding 24
Set Margin 24
Set MarginFill "#f3efe7"
Set BorderRadius 12
Set CursorBlink false
Set TypingSpeed 55ms
Set Theme {
  "name": "Stack Light",
  "black": "#1f2933",
  "red": "#c0362c",
  "green": "#2f855a",
  "yellow": "#b7791f",
  "blue": "#2b6cb0",
  "magenta": "#805ad5",
  "cyan": "#0f766e",
  "white": "#f7fafc",
  "brightBlack": "#4a5568",
  "brightRed": "#e53e3e",
  "brightGreen": "#38a169",
  "brightYellow": "#d69e2e",
  "brightBlue": "#3182ce",
  "brightMagenta": "#9f7aea",
  "brightCyan": "#14b8a6",
  "brightWhite": "#ffffff",
  "background": "#fffdf8",
  "foreground": "#1f2933",
  "selection": "#d9e2ec",
  "cursor": "#2b6cb0"
}

Type "cd .vhs/demo/status/repo"
Enter
Type "./stack-demo init --trunk main --remote origin"
Enter
Sleep 2s

Type "./stack-demo track sandbox-clean-base --parent main"
Enter
Sleep 2s

Type "./stack-demo track sandbox-clean-child --parent sandbox-clean-base"
Enter
Sleep 2s

Type "./stack-demo track sandbox-clean-grandchild --parent sandbox-clean-child"
Enter
Sleep 2s

Type "./stack-demo status"
Enter
Sleep 3s
