# WiZ-CLI Control

A Python-based command-line interface for controlling WiZ smart bulbs over a local network using UDP.

## General Function
The tool acts as a controller for WiZ bulbs on a local network. It performs the following primary functions:
*   **Discovery**: Automatically discovers WiZ bulbs on the local network via broadcast.
*   **Control**: Manages light states (ON/OFF), brightness, RGB colors, and white light temperature.
*   **Scenes**: Provides dynamic lighting effects such as flashes and fades using asynchronous coroutines.
*   **Networking**: Handles all communication via UDP packets and maintains state persistence across commands.

## Key Classes

*   **`Bulb`**: Represents a single physical WiZ bulb. Manages its state (on/off, color, brightness, mode) and sends commands via the `UDPHandler`.
*   **`BulbManager`**: A central hub that maintains the list of discovered `Bulb` instances. Provides methods to add, remove, and retrieve bulbs by ID.
*   **`UDPHandler`**: Manages low-level UDP network communication. Implements a thread-safe, worker-thread approach to queue and send commands to the bulbs.
*   **`SceneRunner`**: Manages the execution of lighting scenes (asynchronous coroutines). It ensures that only one scene runs per bulb at a time by managing task cancellation.
*   **`Scenes`**: A collection of predefined lighting effects (e.g., `blink`, `fade_rgb`, `reset`). Methods are decorated with `@scene` to facilitate asynchronous execution through the `SceneRunner`.

## Global Functions

*   **`discover_bulbs(timeout)`**: Broadcasts a registration message to the network to identify reachable WiZ bulbs.
*   **`load_bulbs_from_file(manager, file_path)`**: Imports bulb IP addresses from external `.toml`, `.colors`, `.ini`, or `.txt` files (one IP per line, ignoring comments) and adds them to the manager.
*   **`load_colors_from_file(file_path)`**: Imports custom color definitions (as RGB tuples) from external `.bulbs`, `.ips`, `.ini`, or `.txt` files to extend the built-in color library.
*   **`execute_command(manager, clean_input)`**: Parses and executes user commands against the `BulbManager`. Returns `True` if the program should exit.
*   **`run_cli(manager)`**: Launches the interactive REPL loop, providing the user interface.
*   **`main()`**: Initializes the environment, handles signals (e.g., Ctrl+C), checks for command-line flags, and determines whether to execute in scriptable mode or launch the interactive CLI.

## Scriptable & REPL Behaviour

### REPL (Interactive) Mode
When the script is executed without arguments, it launches an interactive command-line interface. 
*   Displays a list of available commands upon start.
*   Provides a persistent prompt (`wiz-cli:`) for entering commands iteratively.
*   Handles graceful termination via `quit` or keyboard interrupts (`Ctrl+C`).

### Scriptable (Command-line) Mode & Command-Line Flags
The tool supports executing single commands directly from the shell or utilizing specific utility flags:
*   **Help Flag**: Display usage and available commands without launching discovery or REPL:
    *   `./wiz_cli.py -h` or `./wiz_cli.py --help`
*   **Version Flag**: Print the current application version and exit:
    *   `./wiz_cli.py -v` or `./wiz_cli.py --version`
*   **Import Bulbs Flag**: Import bulb lists via command-line flags before running scriptable commands or launching the REPL:
    *   `./wiz_cli.py -i <file_path>` or `./wiz_cli.py --import-bulbs <file_path>`
*   **Direct Commands**: Pass commands as arguments to the script, e.g., `./wiz_cli.py on 0`. The script executes the provided command and terminates immediately after, making it suitable for integration with other automation tools (e.g., shell scripts or cron jobs).
