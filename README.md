# lanparty
A simple virtual LAN for playing games with friends over the internet.

## Building
```sh
make
```

## Usage
Start a server:
```sh
./lanparty
```

Then give the generated server token to the other players.

Connect as a client:
```sh
./lanparty <token>
```

### Options
```text
-v, --verbose       enable verbose debug logging
-c, --copy-token    copy the server token to the clipboard
```

The `--copy-token` option applies when running as a server.

## How it works
`lanparty` creates a virtual network interface on each machine and connects the interfaces through a central server. Network connectivity is provided using [tailcat](https://github.com/tailscale/tailcat), allowing games that support LAN play to communicate as if the players were on the same local network.

## Requirements
* Linux
* Go
* A network connection between the clients and server

## License
See [LICENSE](LICENSE).
