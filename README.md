ssync
===============

Sync code between two machines in the same relative directory, i.e. your GOPATH.

* Automatic watch built-in with fsnotify, use `--watch=false` to turn off
* Ignore files and patterns like `.git` and `bin/` via `.ssyncignore`
* Concise syntax: `ssync mac-mini` or `ssync alex@rpi.local`

Relies on `rsync` for incremental file transfers, and `ssh` for remote access.

Works wherever you have SSH: port-forwarding on your router, inlets-pro TCP tunnels, VPNs, Tailscale, Wireguard, etc

