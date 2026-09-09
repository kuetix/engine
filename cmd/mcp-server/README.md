# kuetix-engine MCP server

Exposes the WSL/SWSL parser + workflow tooling over the Model Context Protocol
(stdio by default; SSE or streamable HTTP with `-http`).

## Which version am I running?

The parser and transition pipeline come from whatever engine build this binary
was compiled from — so if a client rejects `foreach` / `let` / `while`, its MCP
server is an old build. Two ways to check:

```bash
mcp-server --version
#   kuetix-engine v1.2.0
#     engine:     github.com/kuetix/engine v1.2.0
#     build time: 2026-09-07T07:14:12Z
#     go:         go1.26.1
#     vcs:        234d6ec… 2026-09-06T23:34:18Z
```

or, from a connected client, call the **`server_info`** tool — it returns the
same data as JSON plus the active transport, uptime, and the exact list of
tools this build exposes.

`Version` / `engine_version` is `git describe --tags --dirty` of the engine
repo, stamped in by `make build`. A plain `go run` reports the dev default for
`version` and falls back to the short VCS revision for `engine_version`.

## Logging

Structured logs (slog, text) go to **stderr** by default — never stdout, which
is the MCP channel on the stdio transport.

| flag | env | default |
|---|---|---|
| `-log-level debug\|info\|warn\|error` | `KUETIX_MCP_LOG_LEVEL` | `info` |
| `-log-file <path>` | `KUETIX_MCP_LOG_FILE` | *(stderr)* |

At `info` you get: the startup banner (version, engine version, VCS, pid), the
registered tool list, `serving` with the transport, one `tool ok` line per call
with its `ms`, and `tool failed` / `tool returned error` for failures. `debug`
adds a `tool call` line with the argument keys for every call.

```bash
mcp-server --log-level debug --log-file runtime/log/mcp-server.log
```

## Build / install

```bash
make build      # -> ./mcp-server, stamped with the engine version
make install    # -> $GOBIN/mcp-server
```

Point your MCP client config (`.mcp.json`, Copilot `mcp.json`, …) at the
installed binary. After bumping the engine, rebuild + reinstall so the client
picks up the new grammar — confirm with `server_info`.
