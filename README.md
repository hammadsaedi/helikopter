# helikopter

A helicopter flies in your terminal. While it flies, your machine stays awake.

```
curl -fsSL https://raw.githubusercontent.com/hammadsaedi/helikopter/main/install.sh | sh
helikopter
```

It is `caffeinate -d` with a rotor. Press `q` to land.

---

## What it does

- **Flies.** A shaded helicopter over scrolling parallax terrain, with a rotor that
  actually rotates, motion-blurred blades, drifting cloud, rotor downwash and
  blinking navigation lights.
- **Keeps the machine awake.** Holds a real OS wake lock for as long as it runs, on
  macOS, Linux and Windows. Nothing is left behind if it crashes.
- **Makes noise.** A synthesised soundtrack and rotor wash, generated at startup.
  `--silence` turns it all off.
- **Costs almost nothing.** ~1% of one core while animating; 0% in `--idle`.

## Install

**macOS / Linux / FreeBSD**

```sh
curl -fsSL https://raw.githubusercontent.com/hammadsaedi/helikopter/main/install.sh | sh
```

**Windows** (PowerShell)

```powershell
irm https://raw.githubusercontent.com/hammadsaedi/helikopter/main/install.ps1 | iex
```

**From source** — needs Go 1.21+:

```sh
go install github.com/hammadsaedi/helikopter/cmd/helikopter@latest
```

The installers download a static binary for your platform, verify it against the
published `checksums.txt`, and drop it in `~/.local/bin` (or
`%LOCALAPPDATA%\helikopter\bin`). Override with `HELIKOPTER_BIN_DIR`, pin a build
with `HELIKOPTER_VERSION`.

## Usage

```
helikopter                      fly, with sound, keeping the machine awake
helikopter --silence            fly with no sound at all
helikopter --theme night        pick a look
helikopter --theme random       surprise me
helikopter --duration 90m       fly for 90 minutes, then land
helikopter --idle               no animation: just hold the machine awake
helikopter --idle-after 5m      fly for five minutes, then settle into idle
```

### Keys

| key       | does                        |
| --------- | --------------------------- |
| `q`, `^C` | land                        |
| `space`   | pause                       |
| `t`       | next theme                  |
| `m`       | mute / unmute               |
| `i`       | toggle idle                 |
| `+` / `-` | resize the helicopter       |

### Flags

| flag              | default   | does                                              |
| ----------------- | --------- | ------------------------------------------------- |
| `-t, --theme`     | `crimson` | colour theme, or `random`                          |
| `--list-themes`   |           | list the themes and exit                           |
| `-s, --silence`   |           | no sound at all                                    |
| `--no-music`      |           | rotor noise only                                   |
| `--volume`        | `70`      | 0–100                                              |
| `--fps`           | `20`      | frames per second                                  |
| `--size`          | `0.78`    | helicopter width as a fraction of the terminal     |
| `--quality`       | auto      | supersampling, 1–2                                 |
| `--ascii`         |           | ASCII shading instead of half-block pixels         |
| `--color`         | `auto`    | `auto`, `never`, `16`, `256`, `true`               |
| `-d, --duration`  |           | fly this long then exit (`90m`, `2h`, or minutes)  |
| `--idle`          |           | wake lock only, no animation                       |
| `--idle-after`    |           | animate this long, then drop to idle               |
| `--no-caffeine`   |           | do not hold a wake lock                            |
| `--no-display`    |           | let the screen sleep; block system idle sleep only |
| `--seed`          | random    | scenery seed                                       |
| `--snapshot`      |           | print one frame and exit                           |

## Themes

`helikopter --list-themes`

| theme     |                                              |
| --------- | -------------------------------------------- |
| `crimson` | red gunship against a dust-orange dusk (default) |
| `night`   | blacked-out airframe, moonlight and strobes  |
| `sunset`  | warm chrome on a violet-to-amber sky         |
| `arctic`  | search-and-rescue white over a cold blue day |
| `jungle`  | olive drab, low over the treeline            |
| `vapor`   | synthwave: neon grid, pink hull, cyan glass  |
| `matrix`  | phosphor green, everything else black        |
| `mono`    | greyscale, for terminals and purists         |

A theme colours the whole scene, not just the hull — sky gradient, terrain,
cloud, downwash and every material on the airframe. That is why `red` is a theme
and not a flag: crimson only reads well against the right sky.

## Replacing `caffeinate -d`

`helikopter` holds a genuine OS wake lock for its whole lifetime:

| platform | mechanism                                                        |
| -------- | ---------------------------------------------------------------- |
| macOS    | `caffeinate -dim -w <pid>`                                        |
| Linux    | `systemd-inhibit`, falling back to `gnome-session-inhibit`        |
| Windows  | `SetThreadExecutionState` with `ES_SYSTEM_REQUIRED \| ES_DISPLAY_REQUIRED` |

Every mechanism is scoped to the process. On macOS and Linux the lock lives in a
child process tied to our PID, so `kill -9` releases it too — there is no way to
leak a wake lock. There is deliberately no `xset s off -dpms` fallback on Linux:
that mutates global X settings and would survive a crash.

If no mechanism is available the status line says `awake unavailable` rather than
pretending.

```sh
helikopter --idle               # exactly caffeinate -d, and nothing else
helikopter --idle -d 2h         # ...for two hours
helikopter --no-display         # block system sleep but let the screen off
```

## Cost

Measured on an M3 Pro, 14-second run under a pty:

| mode              | CPU    | RSS    |
| ----------------- | ------ | ------ |
| `--idle`          | 0.0%   | 2.4 MB |
| animating, 80×24  | ~1.0%  | 5.2 MB |
| animating, 200×50 | ~4%    | ~6 MB  |

`--idle` blocks on a signal and a timer; it wakes for nothing at all, which is
why it costs the same as `caffeinate` itself.

Animation is kept cheap deliberately:

- The sky gradient and sun are baked once into a background buffer and copied.
- Each cloud band is a ring of pre-shaded columns that scrolls by whole pixels,
  so a steady-state frame evaluates noise for **one new column per band**
  instead of every pixel.
- The fuselage profile is a lookup table; rotor blade positions are computed
  once per frame, not once per pixel.
- Distances are compared squared, so no square roots run in the per-pixel path.
- Only cells that changed are redrawn, and a rendered frame allocates nothing.
- If frames start missing their budget, supersampling drops before frames do.

Turn it down further with `--fps 10`, `--size 0.5`, or `--quality 1`.
`--idle-after 5m` gives you the animation for five minutes and the wake lock for
as long as you leave it running.

## How the helicopter is drawn

There is no ASCII art in this repository. The airframe is a parametric model —
a fuselage profile, a windscreen curve, polygons for the fin and stabiliser,
segments for the skids — rasterised into a material buffer at whatever
resolution the terminal happens to be, then lit with a real light direction.
Each pixel gets a material (hull, canopy, exhaust, rotor…), a shade and a
specular term; the theme decides what colour those become.

That buys three things a pair of ASCII frames cannot:

- **Shading.** The fuselage is treated as a generalised cylinder, so it has a
  lit top, a shadowed belly and a rim highlight.
- **Real rotation.** The rotor is not a cycle of poses. Blade positions are
  computed from an angle and integrated across a shutter interval, which is
  where the motion blur comes from. The tail rotor is a translucent disc with
  blade shadows sweeping round it.
- **Any size.** It is sharp at 80×24 and sharp full-screen, because it is
  resampled rather than scaled.

Output is half-block glyphs (`▀`), two pixels per cell, so pixels come out
square. `--ascii` falls back to a luminance ramp for terminals without Unicode,
and `--color never` (or `NO_COLOR`) drops to plain shading.

To look at the art as pixels rather than escape codes:

```sh
make preview OUT=./preview     # one PNG per theme
```

## Sound

The soundtrack is synthesised into a WAV at startup — a square-wave hook over a
kick, plus a rotor built from noise chopped at the blade-passing frequency,
matched to the animation's 8.6 Hz. Nothing is embedded and nothing is
downloaded. Playback shells out to whatever the host already has:

| platform | uses                                                     |
| -------- | -------------------------------------------------------- |
| macOS    | `afplay`                                                  |
| Linux    | `paplay`, `pw-play`, `aplay`, `ffplay`, `mpv` or `play`   |
| Windows  | PowerShell `SoundPlayer.PlayLooping`                      |

If none is found the status line says `sound unavailable` and it flies on in
silence. `--silence` skips synthesis entirely; `m` pauses by killing the player
process, so muted audio costs nothing.

## Development

```sh
make build      # build ./helikopter
make test       # go test ./... -race
make lint       # gofmt + go vet
make bench      # render benchmarks
make preview    # PNGs of every theme
make fly        # build and run
```

The repository has one dependency, `golang.org/x/term`, for terminal size and
raw mode. Everything else — rendering, audio synthesis, wake locks, colour
quantisation — is standard library.

## History

This started as [43 lines of Python](https://github.com/hammadsaedi/helikopter/tree/legacy)
that alternated two ASCII frames a thousand times. That version is preserved on
the `legacy` branch.

## Licence

MIT
