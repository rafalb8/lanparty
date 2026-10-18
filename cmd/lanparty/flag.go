package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/pflag"
)

type Config struct {
	Verbose    bool
	CopyToken  bool
	ServerMode bool
	Token      string
	Namespace  bool
}

var errHelp = errors.New("help requested")

func parseFlags(args []string) (Config, error) {
	var cfg Config

	flags := pflag.NewFlagSet("lanparty", pflag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: lanparty [flags] [token]\n       lanparty exec [NAME=value ...] <command> [args...]")
		flags.PrintDefaults()
	}
	flags.BoolVarP(&cfg.Verbose, "verbose", "v", false, "enable verbose debug logging")
	flags.BoolVarP(&cfg.CopyToken, "copy-token", "c", true, "copy the server token to the clipboard")
	flags.BoolVarP(&cfg.Namespace, "namespace", "n", false, "isolate the LAN interface for lanparty exec")

	err := flags.Parse(args)
	if err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			return Config{}, errHelp
		}
		return Config{}, err
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
