package main

import (
	"errors"
	"io"
	"os"

	"github.com/spf13/pflag"
)

type Config struct {
	Verbose    bool
	ServerMode bool
	Token      string
}

var errHelp = errors.New("help requested")

func parseFlags(args []string) (Config, error) {
	var cfg Config

	flags := pflag.NewFlagSet("lanparty", pflag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.BoolVarP(&cfg.Verbose, "verbose", "v", false, "enable verbose debug logging")

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
