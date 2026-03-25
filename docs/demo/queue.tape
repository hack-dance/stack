Output docs/demo/queue.gif

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
Set MarginFill "#ecfdf3"
Set BorderRadius 12
Set CursorBlink false
Set TypingSpeed 55ms
Set Theme {
  "name": "Stack Mint",
  "black": "#14213d",
  "red": "#c53030",
  "green": "#2f855a",
  "yellow": "#975a16",
  "blue": "#2b6cb0",
  "magenta": "#6b46c1",
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
  "background": "#f7fffb",
  "foreground": "#14213d",
  "selection": "#d1fae5",
  "cursor": "#2f855a"
}

Type "cd .vhs/demo/queue/repo"
Enter
Type "./stack-demo init --trunk main --remote origin"
Enter
Sleep 2s

Type "./stack-demo track sandbox-queue-ready --parent main"
Enter
Sleep 2s

Type "./stack-demo submit sandbox-queue-ready --yes --no-restack"
Enter
Sleep 6s

Type "./stack-demo queue sandbox-queue-ready --yes"
Enter
Sleep 6s
