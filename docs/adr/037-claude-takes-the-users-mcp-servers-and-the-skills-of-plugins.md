# ADR-037: Claude takes the user's MCP servers and the skills of their plugins

## Status

Accepted

## Context

ADR-010 gives Claude only the read-only built-in tools, starts no MCP server and turns skills off, so that what a pull request says can reach nothing beyond the pull request. Users want more in the conversation about a pull request: an MCP server that searches their past Claude Code sessions (babarot/claude-recall), or the skills of a plugin they use. Adding each tool to gh-triage one by one does not scale; the user should name what to add.

Opening that door has to keep the rest of ADR-010. Measured on the Claude Code these flags were written against:

- Restricted mode stops the settings files, but not the components of a plugin given with `--plugin-dir`: its hooks run commands. A plugin may also declare language servers, which would start over the worktree, where a language server may run the build scripts of the pull request; whether they start was not settled by measuring.
- A skill brings two more ways to run things: hooks in its frontmatter, and shell commands in its body. Its `allowed-tools` grants tools without a prompt, beyond `--allowedTools`, `--permission-mode dontAsk` included. `--tools` still bounds the built-in tools, so it cannot grant one that is not there.
- Enabling skills at all exposes Claude Code's own skills, which change with each version.
- An MCP server started over stdio runs in Claude's working directory, the worktree, and reads what a configuration file there tells it; the `cwd` of the MCP configuration is not honored.

## Decision

- MCP servers are configured in `claude.mcp_servers`, one table per server, in Claude Code's own shape for a server, plus `tools`: the server's tools that may run without a prompt in `-p`, required and named plainly (no patterns). gh-triage looks only at whether a server is started over stdio and at `tools`, and passes the rest as written, so a field Claude Code adds needs no change here. The servers go in a file only the user can read, given to `--mcp-config`; `--strict-mcp-config` stays, so no other server starts.
- A server started over stdio is started in the cache, not in the worktree, through `/bin/sh` changing directory and running the command with its arguments as positional parameters, so nothing in them is parsed by the shell.
- What a server may need of the pull request is in the environment of `claude`: `GH_TRIAGE_OWNER`, `GH_TRIAGE_REPO`, `GH_TRIAGE_NUMBER` and `GH_TRIAGE_URL`, written as `${GH_TRIAGE_URL}` the way a `.mcp.json` does. gh-triage substitutes nothing itself.
- Plugins are configured in `claude.plugins` as their directories, but are never given to Claude Code as they are. gh-triage copies the skills of each, and only those, into a plugin of its own, made for each run of gh-triage in the cache; no other component of a plugin, today's or a future one, reaches Claude Code. Its directory is added for the file tools, so that a skill can read its own files; they still see nothing of the user's other files. A skill whose `allowed-tools` names anything outside what gh-triage allows (the configured built-in tools, `Skill`, and the tools of `claude.mcp_servers`), or that cannot be read, is a config error, so `tools` and `allowed_tools` stay the bound.
- Every run that may use them (the conversation, and Claude Code in the terminal) takes `--settings` with `disableAllHooks`, `disableBundledSkills` and `disableSkillShellExecution`, whether or not anything is configured. Claude Code's own skills are not stopped by name: a list would follow each version, and in restricted mode none of them can write or run a command.
- The runs that take no tools (the triage plan, the release notes) take no server and no skill.

This replaces, in ADR-010's first decision, "`--strict-mcp-config` with an empty `--mcp-config`" and "`--disable-slash-commands`" for the conversation and Claude Code in the terminal: they take the configured servers, and when a skill is configured, skills on. The rest of ADR-010 stands.

Rejected:

- Passing a plugin with `--plugin-dir` as it is, guarded by `--settings`: the settings do not stop a skill's `allowed-tools` or a plugin's language servers, and a component Claude Code adds to plugins later would pass unseen.
- Passing a plugin as it is and refusing the components known to be dangerous: the same, for a component not known yet.
- Placeholders in the server's fields, substituted by gh-triage: a syntax of gh-triage's own, where Claude Code already expands the environment.
- A server table of gh-triage's own design: it would have to follow each kind of server Claude Code learns to start.

## Consequences

- A server the user adds runs with whatever it can reach, and in `-p` the tools in its `tools` can be called on the say of a pull request's text, without a prompt. A server that reads private data, allowed together with `WebFetch`, gives that text a way to send the data out. gh-triage cannot tell which servers read private data, so it does not refuse the combination; the documentation of the setting says so.
- A skill is a copy taken when gh-triage starts: a plugin updated meanwhile is seen at the next start. Its name changes to `gh-triage:<name>`, and two plugins with a skill of one name are a config error.
- A copy is left in the cache when gh-triage does not exit cleanly; `gh triage gc` removes those of runs no longer alive.
- The guard rests on Claude Code's settings and flags as they behave today. A Claude Code that changes them needs this measured again.
