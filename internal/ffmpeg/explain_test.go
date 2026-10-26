package ffmpeg

import "testing"

func TestExplain(t *testing.T) {
	cases := map[string]string{
		"ffmpeg: [h264_nvenc @ 0x1] Driver does not support the required nvenc API version. Required: 13.0 Found: 12.2\n[h264_nvenc @ 0x1] The minimum required Nvidia driver for nvenc is 570.0 or newer\nError while opening encoder": "NVIDIA driver too old, needs 570.0 or newer",
		"ffmpeg: [h264_nvenc @ 0x1] Driver does not support the required nvenc API version. Required: 13.0 Found: 12.2":                                                                                                                 "NVIDIA driver too old, it speaks NVENC 12.2 and this FFmpeg needs 13.0",
		"ffmpeg: [h264_nvenc @ 0x1] Cannot load nvcuda.dll":                                   "No NVIDIA driver found",
		"ffmpeg: [av1_nvenc @ 0x1] No capable devices found":                                  "This GPU can't encode this format",
		"ffmpeg: [h264_nvenc @ 0x1] OpenEncodeSessionEx failed: incompatible client key (21)": "Encoder busy, another app is using it",
		"ffmpeg: Unknown encoder 'h264_nvenc'":                                                "Not in this FFmpeg build",
		"context deadline exceeded":                                                           "Timed out",
		"[h264_nvenc @ 0000020b27f36100] InitializeEncoder failed: invalid param (8): Invalid Level.\n[vost#0:0/h264_nvenc @ 0000020b27f36100] Error while opening encoder - maybe incorrect parameters such as bit_rate, rate, width or height.\n[vf#0:0 @ 000001] Error sending frames to consumers: Invalid argument\n[vost#0:0/h264_nvenc @ 0000020b27f36100] Terminating thread with return code -22 (Invalid argument)\n[out#0/null @ 0001] Nothing was written into output file, because at least one of its streams received no packets.": "InitializeEncoder failed: invalid param (8): Invalid Level.",
		"[vost#0:0/h264_nvenc @ 0000020b27f36100] Terminating thread with return code -22 (Invalid argument)\n[vf#0:0 @ 01] Some filter problem": "Some filter problem",
		"ffmpeg: something odd happened\nConversion failed!": "something odd happened",
	}
	for in, want := range cases {
		if got := Explain(in); got != want {
			t.Errorf("Explain(%q) = %q, want %q", in, got, want)
		}
	}
}
