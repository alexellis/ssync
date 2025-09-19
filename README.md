ssync
===============

Sync code between two machines in the same relative directory, i.e. your GOPATH.

Examples (bq is a remote Linux host, `.` is your local machine i.e. perhaps a MacBook)

All paths are relative to `cwd` on the machine where `ssync` is run.

```bash
# Copy local pwd (arkade) to bq at the same relative path
# ~/go/src/github.com/alexellis/arkade
ssync bq

# Same as above, but in long form
ssync . bq

# If the folder doesn't exist yet, create it and cd to it
# mkdir -p ~/go/src/github.com/alexellis/arkade
# cd ~/go/src/github.com/alexellis/arkade
# "ls" will show an empty directory, now run the pull:
ssync bq .
```

## Use-cases:

1. I'm going to continue work on another machine (one-off sync)
2. I'm editing locally, but need a Linux machine to build on (run a continual sync with fsnotify)

**Here's how to transfer my work to pick it up later:**

Imagine you're working on your Linux desktop:

I'm working on changes in the ssync repo, but I'm about to leave for a trip with my Mac, or maybe I'm going to work from a cafe for the afternoon.

I don't want to push the branch remote, because it's not ready - or it's a mess. Since it's a public repo, maybe I don't actually want to publish those kinds of temporary changes.

```bash
~/go/src/github.com/alexellis/ssync $
```

On my workstation, before I leave, I run:

```bash
~/go/src/github.com/alexellis/ssync $ ssync ae-mba13 --watch=false
```

This then runs an `rsync` from my workstation to my Mac.

**Editing locally, building/deploying remotely**

Now I'm working on my Mac, but I can't compile the code because it requires Linux, and the binary is too large to scp.

The large binary is named "inletsctl" and can be in the current folder, or in `/bin`:

So I create a .ssyncignore file:

```
/bin/
/inletsctl
```

Then I simply `cd` to the directory and run `ssync --watch=true` in a spare terminal.

Every time I save a file in my editor - like vim or VSCode, an increment `rsync` will take place of any changed files.

## Features

* Automatic watch built-in with fsnotify, use `--watch=false` to turn off
* Ignore files and patterns like `.git` and `bin/` via `.ssyncignore`
* Concise syntax: `ssync mac-mini` or `ssync alex@rpi.local`
* Debouncing when files are changed frequently

Relies on `rsync` for incremental file transfers, and `ssh` for remote access.

Works wherever you have SSH: port-forwarding on your router, inlets-pro TCP tunnels, VPNs, Tailscale, Wireguard, etc
