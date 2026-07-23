package godemon

import (
	"os"

	"golang.org/x/term"
)

func stderrIsTerminal() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
}
