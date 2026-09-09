package foundation

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/hurtener/chartworks/internal/clientcli"
)

// CommandWithInput extends the existing server/configuration command without
// changing its callers. Client commands consume only explicitly supplied Pengui
// authority; they do not initialize the store, verifier, workers or a listener.
func CommandWithInput(ctx context.Context, args []string, lookup func(string) (string, bool), stdin io.Reader, stdout, stderr io.Writer, build Build, start Starter) int {
	if len(args) > 0 && args[0] == "client" {
		return clientcli.Command(ctx, args[1:], clientcli.IO{Stdin: stdin, Stdout: stdout, Stderr: stderr, Lookup: lookup, OpenDescriptor: openClientDescriptor})
	}
	return Command(ctx, args, lookup, stdout, stderr, build, start)
}

func openClientDescriptor(fd int) (*os.File, error) {
	if fd < 3 || fd > 1024 {
		return nil, errors.New("client: invalid descriptor")
	}
	// #nosec G115 -- explicit inherited descriptor validated to 3..1024 above.
	file := os.NewFile(uintptr(fd), "chartworks-client-token")
	if file == nil {
		return nil, errors.New("client: descriptor unavailable")
	}
	return file, nil
}
