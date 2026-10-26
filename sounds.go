package main

import (
	"embed"
	"encoding/binary"
	"os"

	"snag/internal/config"
	"snag/internal/winapi"
)

//go:embed build/sounds/*.wav
var soundFS embed.FS

func soundBytes(name string) []byte {
	b, _ := soundFS.ReadFile("build/sounds/" + name + ".wav")
	return b
}

func withVolume(wav []byte, vol float64) []byte {
	if vol >= 0.999 || len(wav) < 44 || string(wav[0:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		return wav
	}
	out := append([]byte(nil), wav...)
	bits := 0
	for i := 12; i+8 <= len(out); {
		id := string(out[i : i+4])
		size := int(binary.LittleEndian.Uint32(out[i+4 : i+8]))
		body := i + 8
		switch id {
		case "fmt ":
			if size >= 16 && body+16 <= len(out) && binary.LittleEndian.Uint16(out[body:]) == 1 {
				bits = int(binary.LittleEndian.Uint16(out[body+14:]))
			}
		case "data":
			if bits != 16 {
				return wav
			}
			end := body + size
			if end > len(out) {
				end = len(out)
			}
			for j := body; j+1 < end; j += 2 {
				v := float64(int16(binary.LittleEndian.Uint16(out[j:]))) * vol
				binary.LittleEndian.PutUint16(out[j:], uint16(int16(v)))
			}
			return out
		}
		i = body + size + size%2
	}
	return wav
}

func saveSound(s config.Settings) []byte {
	if s.SaveSoundName == "custom" && s.CustomSound != "" {
		if st, err := os.Stat(s.CustomSound); err == nil && st.Size() < 4<<20 {
			if b, err := os.ReadFile(s.CustomSound); err == nil {
				return b
			}
		}
	}
	if b := soundBytes(s.SaveSoundName); b != nil {
		return b
	}
	return soundBytes("pop")
}

func playCue(s config.Settings, cue string) {
	if !s.SaveSound {
		return
	}
	var b []byte
	switch cue {
	case "save":
		b = saveSound(s)
	default:
		b = soundBytes(cue)
	}
	if len(b) > 0 {
		winapi.PlayWav(withVolume(b, float64(s.SoundVolume)/100))
	}
}
