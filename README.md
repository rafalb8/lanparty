# lanparty

A virtual LAN for playing games with friends over the internet, powered by
[tailcat](https://github.com/tailscale/tailcat).

## Building

Requires Linux and Go.

```sh
make
```

This builds a static binary and uses `sudo` to grant `CAP_NET_ADMIN` and
`CAP_SYS_ADMIN`. Run `make` after code changes to rebuild and reapply capabilities.

## Usage

Start a server:

```sh
./lanparty
```

Share the generated token with other players. Connect as a client:

```sh
./lanparty <token>
```

### Network namespaces

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
- Games have LAN access only by default. Add `-i` for internet access.
- Restart games after reconnecting. Only one namespace session per user is supported.
- Games run as your user, with lanparty's capabilities cleared.

#### Optional internet access

Add `-i` (`--internet`) on each machine that needs IPv4 internet access:

```sh
./lanparty -n -i
./lanparty -n -i <token>
```

This requires `-n` and uses each player's own internet connection. Launch games
with `lanparty exec` as usual. A second network interface provides internet access.

Host prerequisites:

- Enable host IPv4 forwarding: `sudo sysctl -w net.ipv4.ip_forward=1`.
- Allow traffic from `lpup<PID>` to your internet interface and established
  return traffic in your firewall. Lanparty does not change host forwarding
  settings or firewall policy.
- Use a kernel with nftables NAT support and an IPv4 or loopback DNS resolver.

No external networking helper is required. IPv6 internet and inbound port
forwarding are not supported. Restart lanparty after changing DNS settings.

Normal shutdown removes the session's network resources. After a forced stop,
verify the process has exited, then remove any leftovers with
`sudo ip link delete lpup<PID>` and `sudo nft delete table ip lanparty_<PID>`.
Replace `<PID>` with the stopped process ID.

### Subnets

Use `--subnet` to change the game LAN (default `10.82.45.0/24`). Set the same
IPv4 `/24` on the server and every client:

```sh
./lanparty --subnet 192.168.50.0/24
./lanparty --subnet 192.168.50.0/24 <token>
```

Use `--internet-subnet` to change the local internet link (default
`10.82.46.0/30`). It must be an IPv4 `/30` and must not overlap the game LAN:

```sh
./lanparty -n -i --internet-subnet 192.168.51.0/30 <token>
```

Both subnets are fixed; lanparty does not search for alternatives. Choose ranges
that do not conflict with existing networks. Concurrent internet sessions on one
machine need different internet subnets; different machines can reuse the same one.

### Options

```text
-v, --verbose          enable verbose debug logging
-n, --namespace        isolate the LAN interface for lanparty exec
-i, --internet         enable IPv4 internet in the namespace (requires --namespace)
    --subnet           game LAN subnet (default 10.82.45.0/24)
    --internet-subnet  local internet subnet (default 10.82.46.0/30)
```

## License

See [LICENSE](LICENSE).
