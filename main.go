// The zeroimage tool builds a "from scratch" OCI image archive from a single
// statically linked executable.
//
// The images produced by this tool may not satisfy the requirements of many
// applications. See the zeroimage README for a discussion of the caveats
// associated with this tool.
package main

import "go.alexhamlin.co/zeroimage/internal/cmd"

func main() {
	cmd.Execute()
}
