package launcher

import (
	"errors"
	"os"
	"strings"
)

func prepareCommand(args []string) ([]string, error) {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}

	for len(args) > 0 {
		name, value, assignment := strings.Cut(args[0], "=")
		if !assignment {
			break
		}

		err := os.Setenv(name, value)
		if err != nil {
			return nil, err
		}

		args = args[1:]
	}

	if len(args) == 0 {
		return nil, errors.New("usage: lanparty exec [NAME=value ...] <command> [args...]")
	}

	return args, nil
}
