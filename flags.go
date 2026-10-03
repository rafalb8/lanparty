package main

import (
	"fmt"
	"os"

	"github.com/spf13/pflag"
)

var Verbose bool

func init() {
	pflag.BoolVarP(&Verbose, "verbose", "v", false, "enable tailcat logs")

	pflag.Usage = func() {
		fmt.Printf("Usage: %s [flags] [token]\n", os.Args[0])
		pflag.PrintDefaults()
	}

	pflag.Parse()
}
