package main

import (
	"fmt"
	"os"

	"github.com/spf13/pflag"
)

type Config struct {
	Verbose    bool
	ServerMode bool
	Token      string
}

func parseFlags() Config {
	var cfg Config
	pflag.BoolVarP(&cfg.Verbose, "verbose", "v", false, "enable verbose debug logging")

	pflag.Usage = func() {
		fmt.Printf("Usage: %s [flags] [token]\n", os.Args[0])
		fmt.Println("  Leave [token] blank to run in server mode.")
		pflag.PrintDefaults()
	}

	pflag.Parse()
	args := pflag.Args()

	if len(args) > 1 {
		pflag.Usage()
		os.Exit(1)
	}

	if len(args) == 0 {
		cfg.ServerMode = true
	} else {
		cfg.Token = args[0]
	}

	return cfg
}