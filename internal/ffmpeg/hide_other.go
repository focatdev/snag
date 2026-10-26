//go:build !windows

package ffmpeg

import "os/exec"

const isWindows = false

func hide(cmd *exec.Cmd, low bool) {}
