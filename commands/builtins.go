package commands

import "github.com/veypi/vsh/internal/completionutil"

// IsShellBuiltin reports actual interpreter builtins; registered tool commands
// are separate. Package installers use this to reject shadowed command names.
func IsShellBuiltin(name string) bool { return completionutil.IsBuiltinName(name) }
