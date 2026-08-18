Output assets/demo.gif

Set Shell "zsh"
Set FontFamily "SFMono Nerd Font"
Set FontSize 15
Set Width 1100
Set Height 560
Set Padding 20
Set Theme "Dracula"

Sleep 200ms

Type "# build sh1llwrt from source"
Enter
Sleep 300ms
Type "go build -o ./sh1llwrt ./cmd/sh1llwrt"
Enter
Sleep 1500ms

Type "# ask a sentence — get the command (mock mode, no model download)"
Enter
Sleep 300ms
Type "./sh1llwrt ask --mock 'extract foo.tar.gz to /tmp'"
Enter
Sleep 700ms

Type "./sh1llwrt ask --mock 'resize cat.png to 200x200'"
Enter
Sleep 700ms

Type "./sh1llwrt ask --mock 'list running pods in kube-system'"
Enter
Sleep 700ms

Type "# machine-readable output for scripts"
Enter
Sleep 200ms
Type "./sh1llwrt ask --mock --json 'mirror src to dest over ssh'"
Enter
Sleep 700ms

Type "# wire the ?-prefix into your shell (dry-run preview)"
Enter
Sleep 200ms
Type "./sh1llwrt init --dry-run"
Enter
Sleep 700ms

Type "# m2 (not yet): inline insert + Ctrl-Space fails closed in m1"
Enter
Sleep 200ms
Type "./sh1llwrt ask --mock --insert 'rm -rf /tmp/old'"
Enter
Sleep 700ms

Sleep 800ms
