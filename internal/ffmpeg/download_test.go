package ffmpeg

import "testing"

func TestPickAssetBelow(t *testing.T) {
	assets := []asset{
		{Name: "ffmpeg-master-latest-win64-gpl.zip"},
		{Name: "ffmpeg-n9.0-latest-win64-gpl-9.0.zip"},
		{Name: "ffmpeg-n9.0-latest-win64-gpl-shared-9.0.zip"},
		{Name: "ffmpeg-n8.1-latest-win64-gpl-8.1.zip"},
		{Name: "checksums.sha256"},
	}
	for below, want := range map[int]string{0: "ffmpeg-n9.0-latest-win64-gpl-9.0.zip", 900: "ffmpeg-n8.1-latest-win64-gpl-8.1.zip", 1 << 20: "ffmpeg-n9.0-latest-win64-gpl-9.0.zip"} {
		a, _, err := pickAsset(assets, below)
		if err != nil || a.Name != want {
			t.Errorf("below %d: got %q %v, want %q", below, a.Name, err, want)
		}
	}
	if _, _, err := pickAsset(assets, 801); err == nil {
		t.Error("expected no build below 8.1")
	}
	if v := StableVersion("n8.1-3-g1234-20261020"); v != 801 {
		t.Errorf("StableVersion = %d", v)
	}
	if v := StableVersion("N-121913-gabc"); v != 0 {
		t.Errorf("master StableVersion = %d", v)
	}
}
