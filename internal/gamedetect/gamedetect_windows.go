package gamedetect

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"snag/internal/winapi"
)

type Info struct {
	Exe        string `json:"exe"`
	Name       string `json:"name"`
	IsGame     bool   `json:"isGame"`
	Fullscreen bool   `json:"fullscreen"`
}

var known = map[string]string{
	"robloxplayerbeta.exe":              "Roblox",
	"robloxstudiobeta.exe":              "Roblox Studio",
	"valorant-win64-shipping.exe":       "Valorant",
	"cs2.exe":                           "Counter-Strike 2",
	"fortniteclient-win64-shipping.exe": "Fortnite",
	"r5apex.exe":                        "Apex Legends",
	"r5apex_dx12.exe":                   "Apex Legends",
	"overwatch.exe":                     "Overwatch 2",
	"rocketleague.exe":                  "Rocket League",
	"league of legends.exe":             "League of Legends",
	"dota2.exe":                         "Dota 2",
	"gta5.exe":                          "GTA V",
	"gta5_enhanced.exe":                 "GTA V",
	"rainbowsix.exe":                    "Rainbow Six Siege",
	"rainbowsix_vulkan.exe":             "Rainbow Six Siege",
	"minecraft.windows.exe":             "Minecraft",
	"javaw.exe":                         "Minecraft",
	"eldenring.exe":                     "Elden Ring",
	"cod.exe":                           "Call of Duty",
	"cod24-cod.exe":                     "Call of Duty",
	"marvel-win64-shipping.exe":         "Marvel Rivals",
	"deadlock.exe":                      "Deadlock",
	"project8.exe":                      "Deadlock",
	"tslgame.exe":                       "PUBG",
	"destiny2.exe":                      "Destiny 2",
	"thefinals.exe":                     "The Finals",
	"geometrydash.exe":                  "Geometry Dash",
	"osu!.exe":                          "osu!",
	"terraria.exe":                      "Terraria",
	"hollow_knight.exe":                 "Hollow Knight",
	"rustclient.exe":                    "Rust",
	"escapefromtarkov.exe":              "Escape from Tarkov",
	"helldivers2.exe":                   "Helldivers 2",
	"cyberpunk2077.exe":                 "Cyberpunk 2077",
	"bf6.exe":                           "Battlefield 6",
	"bf2042.exe":                        "Battlefield 2042",
	"warframe.x64.exe":                  "Warframe",
	"pathofexile_x64steam.exe":          "Path of Exile",
	"pathofexile.exe":                   "Path of Exile",
	"left4dead2.exe":                    "Left 4 Dead 2",
	"hl2.exe":                           "Half-Life 2",
	"vrchat.exe":                        "VRChat",
	"among us.exe":                      "Among Us",
	"lethal company.exe":                "Lethal Company",
	"repo.exe":                          "R.E.P.O.",
	"palworld-win64-shipping.exe":       "Palworld",
	"fallguys_client_game.exe":          "Fall Guys",
	"brawlhalla.exe":                    "Brawlhalla",
	"halo infinite.exe":                 "Halo Infinite",
	"stardew valley.exe":                "Stardew Valley",
	"forzahorizon5.exe":                 "Forza Horizon 5",
	"sotgame.exe":                       "Sea of Thieves",
	"deadbydaylight-win64-shipping.exe": "Dead by Daylight",
	"arc raiders.exe":                   "ARC Raiders",
	"pioneergame.exe":                   "ARC Raiders",
	"splitgate-win64-shipping.exe":      "Splitgate",
	"strinova.exe":                      "Strinova",
	"xdefiant.exe":                      "XDefiant",
	"blackops6.exe":                     "Black Ops 6",
}

var ignored = map[string]bool{
	"explorer.exe": true, "chrome.exe": true, "msedge.exe": true, "firefox.exe": true, "brave.exe": true,
	"opera.exe": true, "opera_gx.exe": true, "vlc.exe": true, "mpc-hc64.exe": true, "mpv.exe": true,
	"potplayermini64.exe": true, "snag.exe": true, "applicationframehost.exe": true, "searchhost.exe": true,
	"shellexperiencehost.exe": true, "startmenuexperiencehost.exe": true, "lockapp.exe": true,
	"discord.exe": true, "spotify.exe": true, "code.exe": true, "devenv.exe": true, "obs64.exe": true,
	"medal.exe": true, "steamwebhelper.exe": true, "textinputhost.exe": true, "zen.exe": true,
}

var (
	mu   sync.Mutex
	last Info
	seen time.Time
)

var shipping = regexp.MustCompile(`(?i)[-_ ]?(win64[-_ ]shipping|win64|x64|dx11|dx12|vulkan|client|shipping|launcher)$`)

func prettify(exe string) string {
	base := strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe))
	for {
		next := shipping.ReplaceAllString(base, "")
		if next == base || next == "" {
			break
		}
		base = next
	}
	base = strings.NewReplacer("_", " ", "-", " ").Replace(base)
	if base == "" {
		return "Desktop"
	}
	return strings.ToUpper(base[:1]) + base[1:]
}

func Current() Info {
	fg, err := winapi.Foreground()
	if err != nil || fg.Exe == "" || fg.PID == os.Getpid() {
		return Info{Name: "Desktop"}
	}
	exe := strings.ToLower(filepath.Base(fg.Exe))
	info := Info{Exe: exe, Fullscreen: fg.Fullscreen}
	if fg.Class == "WorkerW" || fg.Class == "Progman" || fg.Class == "Shell_TrayWnd" {
		return Info{Name: "Desktop"}
	}
	if name, ok := known[exe]; ok && name != "" {
		info.Name, info.IsGame = name, true
		if exe == "javaw.exe" && !strings.Contains(strings.ToLower(fg.Title), "minecraft") {
			info.Name, info.IsGame = prettify(fg.Exe), fg.Fullscreen
		}
	} else if !ignored[exe] {
		info.Name = prettify(fg.Exe)
		info.IsGame = fg.Fullscreen
	} else {
		info.Name = prettify(fg.Exe)
	}
	if info.IsGame {
		mu.Lock()
		last, seen = info, time.Now()
		mu.Unlock()
	}
	return info
}

// alt-tab then save should still name the game
func Recent(window time.Duration) Info {
	cur := Current()
	if cur.IsGame {
		return cur
	}
	mu.Lock()
	defer mu.Unlock()
	if !seen.IsZero() && time.Since(seen) < window {
		return last
	}
	return cur
}
