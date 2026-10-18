# lanparty

A virtual LAN for playing games with friends over the internet, powered by
[tailcat](https://github.com/tailscale/tailcat).

## Building

Requires Linux and Go.

```sh
make
```

This builds a static binary and uses `sudo` to grant `CAP_NET_ADMIN` and
`CAP_SYS_ADMIN`. Reapply capabilities after rebuilding. Keep the static build
to preserve environment variables such as `LD_LIBRARY_PATH` when launching games.

## Usage

Start a server:

```sh
./lanparty
```

Share the generated token with other players. Connect as a client:

```sh
./lanparty <token>
```

### Running games in a network namespace

If a game selects the wrong network interface, start lanparty with `-n`
(`--namespace`):

```sh
./lanparty -n
./lanparty -n <token>
```

Launch the game with:

```sh
./lanparty exec <command> [args...]
```

Steam launch options: `lanparty exec %command%` (use the full path if needed).

`exec` inherits your environment and accepts overrides, without invoking a shell:

```sh
./lanparty exec FOO=bar game --flag
```

- Without an active namespace session, `exec` runs the game normally.
- Namespace games have **LAN access only**, with no internet or online authentication.
- Games must use `exec` to access the isolated LAN. Restart them after reconnecting.
- One namespace session per user. Games run as your user with lanparty's capabilities cleared.

### Options

```text
-v, --verbose       enable verbose debug logging
-c, --copy-token    copy the server token to the clipboard
-n, --namespace     isolate the LAN interface for lanparty exec
```

The `--copy-token` option applies when running as a server.

## License

See [LICENSE](LICENSE).
