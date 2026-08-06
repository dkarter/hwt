---
title: Shell completions
description: Configure hwt completions for Bash, Zsh, Fish, and PowerShell.
---

hwt can generate completion scripts for Bash, Zsh, Fish, and PowerShell.

## Bash

Install and initialize [`bash-completion`](https://github.com/scop/bash-completion) first. Cobra's generated Bash script depends on it.

Load completions in the current shell:

```bash
source <(hwt completion bash)
```

Install them for future sessions:

```bash
mkdir -p ~/.local/share/bash-completion/completions
hwt completion bash > ~/.local/share/bash-completion/completions/hwt
```

## Zsh

Add a completion directory to `fpath` in `~/.zshrc`:

```zsh
fpath=(~/.zfunc $fpath)
autoload -Uz compinit
compinit
```

Then generate the completion file:

```zsh
mkdir -p ~/.zfunc
hwt completion zsh > ~/.zfunc/_hwt
```

Start a new shell or run `exec zsh` after installing it.

## Fish

```fish
mkdir -p ~/.config/fish/completions
hwt completion fish > ~/.config/fish/completions/hwt.fish
```

Fish discovers the file automatically in new and existing sessions.

## PowerShell

Load completions in the current session:

```powershell
hwt completion powershell | Out-String | Invoke-Expression
```

Add the same command to `$PROFILE` to load completions in future sessions.
