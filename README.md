# wiz-cli

`wiz-cli` is a command-line interface to control WiZ smart lights.

The code originally started in Python 3 and has recently been ported to
Go. You can find both the last .py and current .go source files in the
repo.

This project is a result of my hyperfocus, deep love of colorful lights,
and so-far life long journey to find RGB lights that I can set custom
color fades on just the way I want them to be. Also the official WiZ App
is just not it in my opinion... no shade.

------------------------------------------------------------------------

## Features

-   **Automatic Bulb Discovery:** Scans your local Wi-Fi network via UDP
    broadcast to detect WiZ lightbulbs automatically.
-   **Granular Light Controls:** Toggle power (`ON`/`OFF`), set
    brightness (10--100%), adjust color temperature (2200K--6500K),
    apply named colors, or custom RGB values.
-   **Interactive Pickers:**
    -   **fzf Terminal Picker:** Interactively filter and select named
        colors or scenes directly in your terminal.
    -   **macOS Native Color Picker:** Launch the native macOS color
        picker UI from the Terminal (using OSAS) to visually choose
        exact RGB values.
-   **Dynamic Scene Engine:** Run looping background scenes, smooth
    RGB/color transitions (`fade_rgb`, `fade_color`), or custom
    scriptable scenes defined in `.toml` or `.scenes` files.
-   **Persistent Background Daemon:** Argument-based commands are sent
    to a local background daemon, allowing fades and scenes to keep
    running after the shell command exits.
-   **Non-Blocking Architecture:** Scene animations run asynchronously
    using Go channels and context cancellation. The daemon keeps a
    persistent `BulbManager` and `SceneRunner`, so later CLI commands
    can modify or stop already-running scenes.

## Platform & Dependencies

**Platform:** Currently macOS only. Linux integration planned, should
only need minimal refactoring.

**Dependencies:** Requires [`fzf`](https://github.com/junegunn/fzf) for
interactive color and scene pickers. I absolutely love this tool, fzf
for the win! If you're on macOS, you can install fzf via Homebrew:
`bash   brew install fzf`

## Installation & Building

**Download pre-built binary** You can download pre-built binaries and
the default config files from the "Releases" section.

**Building from Source** with Go Compiler Alternatively, you can use the
provided *Build Script* - it builds Intel (amd64), Apple Silicon
(arm64), and Universal macOS binaries (adjust to your needs)

Download the repo, and then run
`bash
cd WiZ-CLI/wiz-cli-go_v6.0/src
chmod +x build-daemon.command
./build-daemon.command`

## Usage

`wiz-cli` can be used in **interactive shell mode** or by **passing
commands directly as arguments**.

*tip: set static IPs for your WiZ bulbs in your router's settings for
your convenience.*

### Interactive Mode

Launch without arguments to start the interactive shell:

``` bash
./wiz-cli
```

In interactive mode, bulbs are automatically discovered on the local
network unless configured otherwise.

### Direct Command / Daemon Mode

When a command is passed directly as arguments, `wiz-cli` automatically
uses its persistent background daemon:

``` bash
./wiz-cli --ips 192.168.1.50,192.168.1.51 fade_color 2 lime yellow 60
```

The CLI sends the command to the daemon and immediately returns control
to your shell. The daemon stays alive in the background and continues
sending the UDP commands required for fades and scenes.

For example:

``` bash
# start a continuous 60-second lime/yellow fade on bulb 2
./wiz-cli --ips 192.168.1.50,192.168.1.51 fade_color 2 lime yellow 60

# the shell prompt returns immediately; the fade keeps running

# change bulb 2 to purple
./wiz-cli color 2 purple

# or explicitly stop its active scene
./wiz-cli stop_scene 2
```

The daemon retains the registered bulb list, so after supplying `--ips`
once you normally do not need to repeat it for subsequent commands.

If no daemon is currently running, the first direct command starts one
automatically.

### Daemon Management

Check whether the background daemon is running:

``` bash
./wiz-cli daemon status
```

Stop the daemon and cancel all active scenes:

``` bash
./wiz-cli daemon stop
```

The daemon communicates with CLI invocations through a local Unix domain
socket in `/tmp`. It is local to the current macOS user and does not
expose a network service.

> **Note:** interactive commands that require direct terminal
> interaction, such as `fzf` or the macOS color picker, are best used
> from interactive shell mode. Direct argument mode is intended
> primarily for fully specified commands and scripting.



## Command Line Flags

| Flag | Description | Example |
|---|---|---|
| `-i`, `--ips` | Specify comma-separated bulb IP addresses. In direct command mode these are registered with the daemon. | `./wiz-cli --ips 192.168.1.10,192.168.1.11 on all` |
| `-f`, `--file` | Load bulb IP addresses from a file. | `./wiz-cli --file bulbs.txt on all` |
| `-v`, `--version` | Display version information and exit. | `./wiz-cli -v` |
| `-h`, `--help` | Show command usage and exit. | `./wiz-cli -h` |


## Daemon Commands

| Command | Description |
|---|---|
| `daemon status` | Show whether the persistent background daemon is currently running. |
| `daemon stop` | Cancel all active scenes and stop the background daemon. |



## Command Reference

The following commands are available both inside the interactive
`wiz-cli` shell and, where no interactive picker is required, as direct
command-line arguments.

In direct command mode, commands are executed by the persistent daemon.
This means scenes started by one invocation can be stopped or replaced
by a later invocation.

Commands can be abbreviated, which allows you to work super fast with
the shell. Different abbreviations are possible; the most convenient
ones are added (in parenthesis) after the commands in the table below.



**General & Management**

| Command | Description |
|---|---|
| `list` (`ls`, `l`) | List all currently registered/discovered bulbs and their state. |
| `discover` (`ds`, `d`) | Trigger a fresh UDP network search for WiZ bulbs. |
| `add <ip1,ip2,...>` | Manually add one or more bulbs by IP address. |
| `poll <id>` | Query a bulb for its raw device status via UDP. |
| `quit` (`exit`, `q`) | Exit the CLI. |



**Power & Brightness**

| Command | Description |
|---|---|
| `on <id\|all>` | Turn a specific bulb ID or all bulbs **ON**. |
| `off <id\|all>` | Turn a specific bulb ID or all bulbs **OFF**. |
| `brightness <id> <10-100>` (`b`) | Set brightness percentage. |



**Color & Temperature**

| Command | Description |
|---|---|
| `color <id> [color_name]` (`c`) | Set a bulb to a named color preset (e.g. `red`, `teal`, `coral`). Opens the `fzf` color selector if no color is specified. |
| `fzf_color <id>` (`fzfc`) | Browse and switch colors continuously using `fzf` until `ESC` is pressed. |
| `color_picker <id>` (`cp`, `picker`) | Open the native macOS GUI color picker. |
| `rgb <id> <R,G,B>` | Set exact RGB values, e.g. `rgb 1 255,128,0`. |
| `temp <id> <2200-6500>` (`t`) | Set light temperature in Kelvin. |
| `color_list` (`cl`) | List all available color presets. |



**Scenes & Animations**
*(note that the 'scenes' in the WiZ App
terminology denote something quite different than here afaik)*

| Command | Description |
|---|---|
| `scene <id> [scene_name]` (`s`) | Run a scene on a bulb. Opens the `fzf` scene selector if no scene is provided. |
| `stop_scene <id>` (`x`) | Stop any running background scene or fade animation on a bulb. |
| `fade_rgb <id> <R1,G1,B1> <R2,G2,B2> [cycle_sec]` | Continuously fade between two RGB colors. Use macOS color picker if no RGB values are specified. |
| `fade_color <id> <color1> <color2> [cycle_sec]` | Continuously fade between two named color presets. Use `fzf` color picker if no RGB values are specified. |
| `reset <id>` | Reset a bulb to a default static state. |
| `color_list` (`cl`) | List all available named colors. |
| `scene_list` (`sl`) | List all loaded custom scenes. |



**Custom Configurations**

| Command | Description |
|---|---|
| `load_scene_file [path]` | Load custom SceneScripts. Defaults to `scenes.toml`. |
| `load_color_file <path>` | Load custom color definitions. |



## Custom Scene & Color Files

Custom scenes and expanded color lists can be written in SceneScript, a
TOML-based DSL.

This is where I believe the true strength of this project lies, as it
gives you full control and customization for things like fades of any
speed between any amount colors, linear or not, pulsing, blinking,
strobes, timers, and so on.

A still somewhat experimental but fully working visual SceneScript
editor written in Python / PySide6 can be found in the tools section.
With this you can easily create your own scenes, using a GUI to create
and preview your scenes graphically, and store them in files for the
main CLI to use (note the requirements.txt).

Alternatively the SceneScript is easy enough to understand to be written
by hand as well.

You can also specify custom color names and import them to the main CLI.

Refer to the provided templates/default files in dist/ for the syntax
for both scenes and colors.


## Feedback and Collaboration

This is one of my very first projects that is actually working and
useful, so I would be very happy to get some feedback or collaboration
on this project, maybe it's interesting for anyone else out there :3

If so, please feel free to get in touch!!


## Further Work

**Reverse Engeneering**
- To me the most interesting next step would be to
look into further features / internals of the bulbs beyond the UDP
networking.

**So Many Colors**
- An obvious and very satisfying avenue would be to build
a rich, extensive and shareable library of well-defined named colors and
especially scenes.

**To Gui Or Not To Gui?**
- Even though the CLI and daemon work incredibly
well and fast for this purpose in my opinion, especially when using
abbreviations in the interactive shell mode, it would still be
interesting to add a GUI layer/wrapper. I've experimented a bit with
[`DearPyGui`](https://github.com/hoffstadt/DearPyGui), an immediate-mode
Python GUI library based on Dear ImGui for this purpose and think it's
very well suited, so this could be an avenue of further development.
Another approach could be Server/Browser based, and could e.g. run on a
Home Server or Pi, and be accessed through the browser from all devices
on the local network. Many possibilities!


## Credits

A big help and inspiration for this project was Sean McNally's
interactive **WiZ UDP Code Generator**:

https://seanmcnally.net/wiz-config.html

Thank you, Sean!

**Icon Credit:** [Flaticon](https://www.flaticon.com/) / Gregor Cresnar


## AI Transparency Disclaimer

I am still somewhat new to coding, but I have found a huge passion for
it.

I started this project in Python, which is the language I know best.
LLMs were used for feedback, troubleshooting, and questions; however,
the entire Python codebase was hand-written.

Since I am very new to Go and still have a lot to learn, more LLM
assistance was involved in the translation to Go. The resulting code was
carefully cross-checked, manually adjusted, and refined.
