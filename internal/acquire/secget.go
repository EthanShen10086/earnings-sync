package acquire

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// secGet retries an SEC archive download with Python.
// www.sec.gov rejects Go and curl as undeclared automated tools, while the
// system Python TLS stack is accepted. The user agent is passed by environment,
// not on the command line.
func secGet(ctx context.Context, rawURL, userAgent string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "python3", "-c", secFetchScript, rawURL)
	cmd.Env = append(os.Environ(), "SEC_FETCH_UA="+userAgent)
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return nil, fmt.Errorf("sec archive refused the download")
	}
	if len(out) > 20<<20 {
		out = out[:20<<20]
	}
	return out, nil
}

const secFetchScript = `
import os, sys, urllib.request
url = sys.argv[1]
req = urllib.request.Request(url, headers={"User-Agent": os.environ.get("SEC_FETCH_UA", ""), "Accept": "*/*"})
with urllib.request.urlopen(req, timeout=45) as res:
    sys.stdout.buffer.write(res.read())
`
