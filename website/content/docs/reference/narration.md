---
title: Narration
weight: 8
---

Narration is everything nem prints on stderr: progress, outcomes, warnings, errors and hints. Data a command produces goes to stdout and is the same in every format.

## The flag and the variable

`--narration auto|plain|json` picks the narration format. The default is `auto`. Setting `NEM_NARRATION` to the same values sets the default, so an agent sets it once per session.

A flag value other than `auto` wins over the variable; `auto` on the flag defers to the variable. An empty variable means `auto`. A bad value, in the flag or in the variable, fails before anything else runs and exits 2:

```
Error: invalid --narration value "xml" (want auto, plain, or json)
```

A bad variable value prints the same error naming the variable:

```
Error: invalid NEM_NARRATION value "xml" (want auto, plain, or json)
```

## Modes

| `--narration` | stderr is a terminal | Narration on stderr |
|---|---|---|
| `auto` | yes | live: tasks are repainted in place |
| `auto` | no | plain |
| `plain` | any | plain |
| `json` | any | one JSON object per line |

Plain is a static transcript, with no live block and no cursor movement. It is the one way to get a static transcript on a terminal; `--color never` alone keeps the live block. Plain still follows `--color`.

JSON is never coloured and has no live block, whatever `--color` says.

`--quiet` and `--verbose` filter every format alike. A line or task hidden in plain output is absent from the JSON stream too.

## JSON event stream

Under `--narration json`, stderr carries one JSON object per line, one object per event: UTF-8, no indentation, no ANSI codes, each line ended by a newline.

Every object has `seq` (1 for the first event, one more for each next event), `time`, and `event`. Task events also have `task`, a small integer unique within the run, and `state`.

| `event` | `state` | Other fields |
|---|---|---|
| `begin` | | `app`, `version`, `schema` |
| `line` | | `level`, `text`; `hint` on an error line that has one; `task` when the line comes from a task |
| `task` | `started` | `label` |
| `task` | `phase` | `phase` |
| `task` | `progress` | `phase`, `done`, `total`, `unit` |
| `task` | `done` | `outcome`, `elapsed_ms` |
| `task` | `failed` | `outcome`, `elapsed_ms` |
| `task` | `discarded` | none |
| `output` | | `stream`, `text`; `partial` when true; `task` when written by a task |
| `exit` | | `code`; `signal` when a signal ended the run |

`app` is `nem` and `version` is the running nem version. `level` is one of `info`, `success`, `warn`, `error`, `hint`, `debug`, `prompt`, `answer`. `stream` is `stdout` or `stderr`. `unit` is `items` or `bytes`. A field that does not apply is left out.

## Rules for consumers

- `begin` is the first line and `exit` the last on a normal run. `schema` is 1 and changes only on a breaking change.
- A failure before output is set up still ends in `exit`. A bad flag or argument prints an error `line` and an `exit` with code 2, with no `begin`.
- A stream without `exit` ended abnormally: a second signal that came while output was being written, a crash outside the command itself, or a forced kill.
- Under `--quiet`, a `failed` event can arrive for a task with no `started` event before it; the outcome names its subject, so it stands alone. Do not assume every task begins with `started`.
- A task with no `done`, `failed` or `discarded` event at `exit` was interrupted. No outcome is made up for it.
- Tolerate lines that are not JSON. A crash report, or the output of a passthrough command's child, can appear on stderr.
- Ignore unknown values and fields. An unknown `event`, `state` or `level`, and any unknown field, is skipped. Adding a field or a value is compatible; renaming or removing one is a breaking change and bumps `schema`.
- `time` is RFC 3339 in UTC with microseconds, as in `2026-10-16T09:00:00.123456Z`.
- Progress is sampled about once per second. A `progress` event is sent only when the phase, `done`, `total` or `unit` changed since the last one for that task. None is sent before a phase is set, and none when the unit is items and the total is 0. `total` is -1 when unknown. No final progress event is forced before `done` or `failed`.
- `elapsed_ms` is the raw elapsed time, not the rounded suffix plain output shows.
- An error line's `text` is the display form, the same as plain output.

## Output events

Child output is sent line by line: one `output` event per line, ended by `\n` or `\r`, with the ending kept in `text`. When 8 KiB arrive without a line ending, they are sent as one event with `"partial":true`. Bytes that are not valid UTF-8 are replaced.

Output a task writes, to either stream, becomes `output` events carrying the task's id. The bytes do not also reach stdout or stderr, so a consumer that reads both streams never sees them twice.

Stderr output outside any task becomes `output` events with no `task`. Stdout output outside any task is data and passes through to stdout unchanged.

Commands that hand both streams to a child, such as `nem exec`, cannot keep stderr to one JSON object per line. Their `begin` and `exit` events still mark the start and end of the run.

JSON narration does not change input. A command that prompts still waits for an answer; the stream reports the `prompt` and `answer` lines.

## Signals and exit codes

The first `SIGINT` (Ctrl-C) or `SIGTERM` cancels the running work. Running tasks end with `Cancelled <subject>`.

A command that fails after `SIGINT` exits 130; after `SIGTERM`, 143. The `exit` event then carries `"signal":"SIGINT"` or `"signal":"SIGTERM"`. A command that finishes without an error after a signal exits 0. A cancellation with no signal behind it, such as a timeout, is an ordinary error with exit code 1.

A second signal exits at once, with 130 for `SIGINT` and 143 for `SIGTERM`.

## Example

Run a sync with JSON narration and list the tasks that failed:

```sh
NEM_NARRATION=json nem sync 2>events.jsonl
jq -c 'select(.event == "task" and .state == "failed") | {task, outcome}' events.jsonl
```

Each result carries the task id and its outcome, such as `Failed to install jq 1.8.3`. The last line of `events.jsonl` is the `exit` event; its `code` is the exit code of the run.
