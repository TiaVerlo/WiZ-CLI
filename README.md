# wiz-cli

`wiz-cli` is a command-line interface to control WiZ smart lights.

The project originally started in Python 3 and has recently been ported to Go.

It is a result of my hyperfocus, deep love of colorfull lights, and so-far life long journey to find RGB lights that I can set custom color fades on the way I want them to be.
Also the official WiZ App is just not it in my opinion... no shade.

---

## Features

* **Automatic Bulb Discovery:** Scans your local Wi-Fi network via UDP broadcast to detect WiZ lightbulbs automatically.
* **Granular Light Controls:** Toggle power (`ON`/`OFF`), set brightness (10–100%), adjust color temperature (2200K–6500K), apply named colors, or custom RGB values.
* **Interactive Pickers:** 
  * **fzf Terminal Picker:** Interactively filter and select named colors or scenes directly in your terminal.
  * **macOS Native Color Picker:** Launch the native macOS color picker UI from the Terminal (using) to visually choose exact RGB values.
* **Dynamic Scene Engine:** Run looping background scenes, smooth RGB/color transitions (`fade_rgb`, `fade_color`), or custom scriptable scenes defined in `.toml` or `.scenes` files.
* **Non-Blocking Architecture:** Scene animations run asynchronously using Go channels and context cancellation without freezing the CLI.

---

## Platform & Dependencies

* **Platform:** Currently macOS only.
* **Dependencies:** Requires [`fzf`](https://github.com/junegunn/fzf) for interactive color and scene pickers. You can install it via Homebrew:
  ```bash
  brew install fzf
```

## Installation & Building
1. Build from Source

If you have Go installed on your machine:
  ```bash
  go build -o wiz-cli wiz-cli.go
```

2. Build Script - Intel (amd64), Apple Silicon (arm64), and universal macOS Binary
  ```bash
  chmod +x build.sh
  ./build.sh
```
outputs into the dist/ directory

---

## Usage

You can launch wiz-cli in interactive shell mode or pass commands directly as arguments.
*note: It is highly recommended that you set static IPs for your WiZ bulb in your router's settings.*
  ```bash
# Launch interactive mode (auto-discovers bulbs)
./wiz-cli

# Pre-load specific bulb IPs
./wiz-cli --file bulbs.txt
./wiz-cli --ips 192.168.1.50,192.168.1.51

# Run a single command directly
./wiz-cli on all  # --> turns all connected bulbs on
```

## Command Line Flags

| Flag | Description | Example |
|---|---|---|
| `-ips`, `-i`, `--ips` | Specify comma-separated bulb IP addresses on startup. | `./wiz-cli -ips 192.168.1.10,192.168.1.11` |
| `-file`, `-f`, `--file` | Load bulb IP addresses from a file. | `./wiz-cli -file bulbs.txt` |
| `-v`, `--version` | Display version information and exit. | `./wiz-cli -v` |
| `-h`, `--help` | Show command usage and exit. | `./wiz-cli -h` |


## Command Reference

Once inside the `wiz-cli` shell — or when executing one-off commands — the following commands are available.

### General & Management

| Command | Description |
|---|---|
| `list` (`ls`, `l`) | List all currently registered/discovered bulbs and their state. |
| `discover` (`ds`, `d`) | Trigger a fresh UDP network search for WiZ bulbs. |
| `add <ip1,ip2,...>` | Manually add one or more bulbs by IP address. |
| `poll <id>` | Query a bulb for its raw device status via UDP. |
| `quit` (`exit`, `q`) | Exit the CLI. |

### Power & Brightness

| Command | Description |
|---|---|
| `on <id\|all>` | Turn a specific bulb ID or all bulbs **ON**. |
| `off <id\|all>` | Turn a specific bulb ID or all bulbs **OFF**. |
| `brightness <id> <10-100>` (`b`) | Set brightness percentage. |

### Color & Temperature

| Command | Description |
|---|---|
| `color <id> [color_name]` (`c`) | Set a bulb to a named color preset (e.g. `red`, `teal`, `coral`). Opens `fzf` if no color is specified. |
| `fzf_color <id>` (`fzfc`) | Browse and switch colors continuously using `fzf` until `ESC` is pressed. |
| `color_picker <id>` (`cp`, `picker`) | Open the native macOS GUI color picker. |
| `rgb <id> <R,G,B>` | Set exact RGB values, e.g. `rgb 1 255,128,0`. |
| `temp <id> <2200-6500>` (`t`) | Set light temperature in Kelvin. |
| `color_list` (`cl`) | List all available color presets. |

### Scenes & Animations
*note that the 'scenes' in the WiZ App terminology denote something quite different than here afaik*

| Command | Description |
|---|---|
| `scene <id> [scene_name]` (`s`) | Run a scene on a bulb. Opens the `fzf` scene selector if no name is provided. |
| `stop_scene <id>` (`x`) | Stop any running background scene or fade animation on a bulb. |
| `fade_rgb <id> <R1,G1,B1> <R2,G2,B2> [cycle_sec]` | Continuously fade between two RGB colors. |
| `fade_color <id> <color1> <color2> [cycle_sec]` | Continuously fade between two named color presets. |
| `reset <id>` | Reset a bulb to its default static state (dim warm white). |
| `scene_list` (`sl`) | List all loaded custom scenes. |

### Custom Configurations

| Command | Description |
|---|---|
| `load_scene_file [path]` | Load custom SceneScripts. Defaults to `scenes.toml`. |
| `load_color_file <path>` | Load custom color definitions. |


--- 

## Custom Scene & Color Files
Custom scenes and expanded color lists can be written in SceneScript, a TOML-based DSL. Refer to the provided templates for the syntax.

A still somewhat experimental but working visual SceneScript editor written in Python / PySide6 can be found in the tools section. This allows you to create your own scenes and store them in files for the main CLI to use.

---

## Feedback and Collaboration
I would be so happy to get some feedback or collaboration on this project, maybe it's interesting for anyone else out there :3
If so, please get in touch!!


## Further Work
*Reverse Engeneering*
To me the most interesting next step would be to look into further features / internals of the bulbs beyond the UDP networking.

*So Many Colors*
An obvious and very satisfying avenue would be to build a rich, extensive and shareable library of well-defined named colors and especially scenes.

*To Gui Or Not To Gui?*
Even though the CLI works incredibly well and fast for this purpose in my opinion, especially when using abbreviations in the interactive shell mode, it would still be interesting to add a GUI wrapper.
I've experimented a bit with DearPyGui, an immediate-mode Python GUI library based on Dear ImGui for this purpose and think it's very well suited, so this could be an avenue of further development.
Another approach could be Server/Browser based, and could e.g. run on a Home Server or Pi, and be accessed through the browser from all devices on the local network. Many possibilities!


## Credits
A big help and inspiration for this project was Sean McNally's interactive **WiZ UDP Code Generator**:

https://seanmcnally.net/wiz-config.html

Thank you, Sean!

**Icons:** [Flaticon](https://www.flaticon.com/) / Gregor Cresnar

---

## AI Disclaimer
I am still somewhat new to coding, but I have found a huge passion for it.

I started this project in Python, which is the language I know best. LLMs were used for feedback, troubleshooting, and questions; however, the entire Python codebase was hand-written.

Since I am very new to Go and still have a lot to learn, more LLM assistance was involved in the translation to Go. The resulting code was carefully cross-checked, manually adjusted, and refined.

