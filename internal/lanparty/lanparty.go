package lanparty

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/tailscale/tailcat"
	"golang.design/x/clipboard"
)

const (
	controlPort = 8245
	dataPort    = 8246

	serverCIDR = "10.82.45.1/24"

	maxBufferSize = tailcat.MaxUDPPayload
)

func printInfo(wr io.Writer, title string, rows ...string) {
	if title != "" {
		fmt.Fprintln(wr)
		fmt.Fprintln(wr, title)
		fmt.Fprintln(wr, "──────────────")
	}

	w := tabwriter.NewWriter(wr, 0, 4, 1, ' ', 0)
	for i := 0; i < len(rows); i += 2 {
		fmt.Fprintf(w, "%s\t%s\n", rows[i], rows[i+1])
	}
	w.Flush()

	if title != "" {
		fmt.Fprintln(wr)
	}
}

func copyToClipboard(token string) error {
	err := clipboard.Init()
	if err != nil {
		return err
	}

	_, err = clipboard.Write(context.Background(), clipboard.FmtText, []byte(token))
	return err
}
