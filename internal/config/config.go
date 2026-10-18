package config

import (
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"

	"github.com/spf13/pflag"
)

type Config struct {
	Verbose        bool
	ServerMode     bool
	Token          string
	Namespace      bool
	Internet       bool
	Subnet         netip.Prefix
	InternetSubnet netip.Prefix
}

const (
	ControlPort = 8245
	DataPort    = 8246
)

var ErrHelp = errors.New("help requested")

func Parse(args []string) (Config, error) {
	var cfg Config
	var subnet, internetSubnet string

	flags := pflag.NewFlagSet("lanparty", pflag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: lanparty [flags] [token]\n       lanparty exec [NAME=value ...] <command> [args...]")
		flags.PrintDefaults()
	}
	flags.BoolVarP(&cfg.Verbose, "verbose", "v", false, "enable verbose debug logging")
	flags.BoolVarP(&cfg.Namespace, "namespace", "n", false, "isolate the LAN interface for lanparty exec")
	flags.BoolVarP(&cfg.Internet, "internet", "i", false, "enable IPv4 internet in the namespace (requires --namespace)")
	flags.StringVar(&subnet, "subnet", "10.82.45.0/24", "game LAN subnet (IPv4 /24; must match on all players)")
	flags.StringVar(&internetSubnet, "internet-subnet", "10.82.46.0/30", "local namespace internet subnet (IPv4 /30)")
	
	err := flags.Parse(args)
	if err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			return Config{}, ErrHelp
		}
		return Config{}, err
	}

	if cfg.Internet && !cfg.Namespace {
		return Config{}, errors.New("--internet requires --namespace")
	}
	cfg.Subnet, err = parseSubnet("--subnet", subnet, 24)
	if err != nil {
		return Config{}, err
	}
	cfg.InternetSubnet, err = parseSubnet("--internet-subnet", internetSubnet, 30)
	if err != nil {
		return Config{}, err
	}
	if cfg.Internet && cfg.Subnet.Overlaps(cfg.InternetSubnet) {
		return Config{}, errors.New("--subnet and --internet-subnet must not overlap")
	}

	args = flags.Args()
	if len(args) == 0 {
		cfg.ServerMode = true
		return cfg, nil
	}

	if len(args) == 1 {
		cfg.Token = args[0]
		return cfg, nil
	}

	_, err = io.WriteString(os.Stderr, "Usage: lanparty [flags] [token]\n")
	if err != nil {
		return Config{}, err
	}

	return Config{}, errors.New("expected at most one server token")
}

func parseSubnet(flag, value string, bits int) (netip.Prefix, error) {
	subnet, err := netip.ParsePrefix(value)
	if err != nil || !subnet.Addr().Is4() || subnet.Bits() != bits {
		return netip.Prefix{}, fmt.Errorf("%s must be an IPv4 /%d subnet", flag, bits)
	}
	return subnet.Masked(), nil
}
