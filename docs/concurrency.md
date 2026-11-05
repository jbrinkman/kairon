# Agent concurrency

Kairon runs at most `max_agents` agents at once. The limit covers implementation runs, retries and intake runs. When
more issues are ready than there are free slots, the oldest issues start first and the rest wait for a later poll.

The setting is read in `internal/config`. The slots are kept by the agent manager (`internal/agent`), and the watcher
(`internal/watcher`) asks the manager how many are taken before it starts an issue.

## The setting

```yaml
# .kairon/config.yaml
max_agents: 3
```

| Property | Value |
|----------|-------|
| Default | `3`, used when the key is absent |
| Valid values | Any positive integer |
| Zero or negative | Rejected when the config loads, so Kairon does not start. The error reads `max_agents must be greater than 0 (got N); there is no unlimited setting` |
| Unlimited | Not configurable. No value removes the limit |

`kairon init` writes `max_agents: 3` with a comment into a new `.kairon/config.yaml`. `kairon init` and `kairon update`
leave an existing `config.yaml` alone, so a project created earlier has no `max_agents` line and runs with the default
of 3 until you add one.

## What holds a slot

A slot belongs to an issue, and an issue holds at most one. A second start for an issue that already holds a slot is
refused. The manager takes the slot while holding its lock, before it creates a worktree, a log file or a process, so
two starts cannot both take the last slot.

| Run | Slot taken | Slot released |
|-----|-----------|---------------|
| Implementation (krew-lead) | When the issue is admitted and the run starts, before its worktree is created | The agent exits 0 and a pull request exists. The agent fails and no retries remain, which is a punch-out. The run fails to start after the slot was taken (worktree, log file or process). |
| Retry | Not taken again. The retry keeps the slot of the run it retries | The retry ends without starting a new process: the issue is no longer admitted, or the input document, worktree or log file cannot be set up, or an intake run took the issue, or the process fails to start. Otherwise the same as an implementation run. |
| Intake | When the intake run starts, before it reads the issue revision or starts its planner process | The run ends, however it ends: committed, punched out, the issue changed during the run, stopped, or failed. |

Three consequences follow from these rules:

- **A failed agent that will be retried still holds its slot.** After a non-zero exit, `status` shows the agent as
  failed while the manager waits `RetryCount` seconds, re-checks the issue and restarts the process. The slot is held
  for that whole gap, so another issue cannot start in it and push the count past the limit when the retry starts.
  An agent that exits 0 without a pull request is treated as a failure and follows the same path.
- **An intake run keeps its slot between its own attempts.** A rejected planner reply is retried up to `max_retries`
  times, and the slot is held until the run ends.
- **An issue that is skipped, closed or punched out at admission never takes a slot.** The slot is reserved only after
  the issue is admitted.

An issue without a provenance marker needs a slot for its intake run. When that run commits and releases the slot, the
next poll admits the issue and it needs a slot again for the implementation run, so it can wait at that point.

## Oldest first

Each poll sorts the eligible issues by issue number, lowest first, and goes through them in that order. For every issue
the watcher applies its existing checks before it looks at capacity:

1. It skips an issue that is already tracked, has an active worktree, has an intake run in progress, has used up its
   `max_retries`, is in dependency backoff, or has an open dependency.
2. It checks for a free slot. If there is none, the issue waits.
3. Otherwise it starts the issue.

Because the capacity check is last, an older issue that is blocked (for example by an open dependency) does not take a
slot and is not reported as waiting. The next-oldest issue that could start does. The free count is read again before
each issue, so when the first issue of a poll takes the last slot, the remaining issues wait in that same poll.

Once an issue waits, every younger issue that could start waits too for the rest of that poll, even if a run ends and
frees a slot before the watcher reaches it. The freed slot goes to the older waiting issue on the next poll, so a
younger issue never starts ahead of an older one that is waiting.

A freed slot is used by the next poll, not immediately. With the default `poll_interval` of `5m`, a waiting issue can
start up to five minutes after a slot frees.

## Waiting issues and the retry budget

An issue that waits for a slot is not charged a retry attempt. The watcher checks for a free slot before it marks the
issue as tracked and before it increments the retry count in `.kairon/retries/issue-<number>.count`, so the count is
unchanged and the next poll considers the issue again.

`max_retries` bounds failed runs only. A long wait for a slot never moves an issue toward `<label>-failed`.

If an issue passes the watcher's check and the manager then finds no free slot (for example `validate` took the last
slot in between), the watcher untracks the issue and refunds the attempt, so the count again ends unchanged.

## The log line

Each poll writes one line per waiting issue, at info level:

```
[watcher] INFO: issue #N waiting for an agent slot (M/M running)
```

`N` is the issue number. The first `M` is the number of slots taken and the second `M` is `max_agents`. An issue waits
only when every slot is taken, so both numbers are normally the same (`3/3` with the default). The line is written once
per poll for as long as the issue keeps waiting. It is not a failure: the issue gets no comment and no
`<label>-failed` label, and the line has no `WARNING:` or `ERROR:` tag. The watcher writes it with the standard `log`
package, like the rest of its output.

## Validate at capacity

`validate <issue>` runs an intake run, so it needs a slot. When none is free, `validate` reports an error, starts no
run and writes nothing to the issue. The console shows:

```
Validate #N failed: agent: no free agent slot (M/M running)
```

Run `validate` again after a slot frees. `validate` does not queue and does not wait.

Results that do not start a run are reported as before and need no slot: the issue is already valid, the issue is
closed or done, or a run for the issue is already going.

## Runs left by a previous watcher

An intake or implementation run can outlive the Kairon process that started it, for example when Kairon crashes or is
killed. Such a run is known only by its PID marker, which holds the agent's process ID while that process runs:
`.kairon/intake/issue-<number>.pid` for an intake run and `.kairon/agents/issue-<number>.pid` for an implementation
run. A new Kairon counts each issue with a marker whose process is still alive as one taken slot, unless the issue is
already in its own slot table, so a run is never counted twice. A marker whose process is dead is stale: it is removed
and does not count. When the process ends, its marker is removed and the slot is free.

Without this count, a restart could run more than `max_agents` agents. If you lower `max_agents` while such runs are
still going, the first number in the log line can be larger than the second (for example `4/3 running`), and nothing
new starts until enough runs end.

## Changing the limit

1. Set `max_agents` in `.kairon/config.yaml` to a positive integer.
2. Restart Kairon. The config is read once at startup, so a running Kairon keeps the old limit.

Exiting Kairon stops the agents it is running. Check `status` first, and wait until the agents you care about have
finished. `exit` asks for confirmation while agents are still running.

A higher value runs more issues in parallel. Each running issue is a separate `kiro-cli` agent: an implementation run
works in its own worktree, and an intake run works in the directory Kairon was started from.
