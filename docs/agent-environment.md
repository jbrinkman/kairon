# How agents find `kairon`

Agent prompts run `kairon` subcommands, for example `kairon scope check` and `kairon plan parse` in krew-lead. An agent
always reaches **the `kairon` binary file that started Kairon** (the path `os.Executable()` reports, symlinks
resolved), whether or not any `kairon` is installed on your `PATH`.
Running the watcher from a build that is not installed (for example `./dist/kairon`) works the same as running an
installed one.

## How it works

When the watcher and TUI start (`kairon` with no subcommand), Kairon:

1. Resolves the running binary with `os.Executable()`, following symlinks.
2. Creates `.kairon/bin/` under the project root if needed. The directory holds one entry, a `kairon` symlink to the
   running binary. Kairon replaces the symlink at every start, so a link left by an older binary is updated. Nothing
   else from the binary's own directory is exposed.

The link names the binary's path, not a copy. If you rebuild that file in place while Kairon is running (for example
`task build` over `./dist/kairon`), agents started afterwards run the new build; restart Kairon to bring the watcher
itself up to date.

Every `kiro-cli` agent process Kairon starts then gets this environment:

| Variable | Value |
|----------|-------|
| `PATH` | `<project root>/.kairon/bin` first, then the `PATH` Kairon itself was started with |
| `KAIRON_BIN` | The absolute, symlink-resolved path of the running `kairon` binary. Any inherited value is replaced. |

This applies to the implementation agent (krew-lead and the sub-agents it starts), the automated planner (intake), the
interactive classic planner, ACP planning sessions and planner session processes.

If Kairon cannot resolve its own binary or create the symlink, it stops at startup with an error starting
`exposing kairon to agents:` instead of starting agents that cannot find it.

The eval runner is separate: it builds its own `PATH` and mounts a linux `kairon` into sandbox containers (see
[evaluation.md](evaluation.md)).

## Version control

`.kairon/bin/` is machine-specific and must not be committed. `kairon init` adds `.kairon/bin/` to the project's
`.gitignore`, creating the file if needed and leaving existing entries untouched. In an existing project that has not
re-run `kairon init`, add the line yourself:

```
.kairon/bin/
```
