# sh1llwrt — what using it looks like (3 lines).
go install ./cmd/sh1llwrt
sh1llwrt init && exec "$SHELL"            # wire the ?-prefix; restart your shell
? extract foo.tar.gz to /tmp             # → tar -xzf foo.tar.gz -C /tmp
