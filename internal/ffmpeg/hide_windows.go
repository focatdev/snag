package ffmpeg

import (
	"os/exec"

	"snag/internal/winapi"
)

const isWindows = true

func hide(cmd *exec.Cmd, low bool) { winapi.HideConsole(cmd, low) }
