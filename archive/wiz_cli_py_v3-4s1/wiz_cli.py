#!/usr/bin/env python3
# -*- coding: utf-8 -*-

# requires Python 3.9+

import os
import sys
import struct
import signal
import subprocess
import asyncio
import threading
import time
import socket
import json


GLOBAL_SUBLIME_SAVECOUNT = True  # sublime flag: auto-increase version
__version__ = "3.4s1"


# ======== ANSI COLOR CODES FOR CLI ========
RESET = "\033[0m"
BOLD = "\033[1m"
UNDERLINE = "\033[4m"

# fg font colors
FG_BLACK = "\033[30m"
FG_RED = "\033[31m"
FG_GREEN = "\033[32m"
FG_YELLOW = "\033[33m"
FG_BLUE = "\033[34m"
FG_MAGENTA = "\033[35m"
FG_CYAN = "\033[36m"
FG_WHITE = "\033[37m"
FG_BRIGHT_CYAN = "\033[96m"
FG_BRIGHT_GREEN = "\033[92m"
FG_GREY = "\033[90m"

# bg font colors
BG_BLUE = "\033[44m"

# special text
ERROR = f"{BOLD}{FG_RED}ERROR: {RESET}"



# ======== BULB COLORS ========
color_list = {
    # favorites
    'red_pink':     (255,   0,  90),
    'blue_pink':    (255,   0, 170),
    'blue_purple':  ( 95,   0, 255),
    'purple':       (165,   0, 255),
    'yellow_green': (150, 255,   0),
    'green':        ( 30, 255,   0),
    
    # primary
    'red':          (255,   0,   0),
    'yellow':       (255, 255,   0),
    'blue':         (  0,   0, 255),
    
    # red-yellow range
    'orange':        (255, 128,   0),
    'deep_orange':   (255,  69,   0),
    'amber':         (255, 191,   0),
    'gold':          (255, 215,   0),
    
    # green-blue range
    'turquoise':     ( 64, 224, 208),
    'emerald':       ( 80, 200, 120),
    'teal':          (  0, 128, 128),
    'cyan':          (  0, 255, 255),
    'lightblue':     (135, 206, 235),
    'lime':          (  0, 255,   0),
    'forest_green':  ( 34, 139,  34),
    'mint':          (152, 251, 152),
    'sky_blue':      (  0, 191, 255),
    'navy':          (  0,   0, 128),
    'indigo':        ( 75,   0, 130),
    
    # purple-pink range
    'magenta':       (255,   0, 255),
    'orchid':        (218, 112, 214),
    'bright_pink':   (255, 105, 180),
    'pink':          (255,  20, 147),
    'violet':        (238, 130, 238),
    'lavender':      (230, 230, 250),
    'plum':          (221, 160, 221),
    'coral':         (255, 127,  80),
}

def load_colors_from_file(file_path: str) -> dict:
    """
    import colors from .toml / .ini / .txt file and merge into color_list
    file format:
        color1 = [R, G, B]
        color2 = [R, G, B]
    [headers] and #comments are ignored
    """
    base_dict = color_list
    ext = os.path.splitext(file_path)[1].lower()
    if ext not in (".toml", ".colors", ".ini", ".txt"):
        print(f"{ERROR}{FG_RED}unsupported file type{RESET}")
        return base_dict
    imported_colors = {}
    with open(file_path, 'r', encoding='utf-8') as f:
        for line in f:
            line = line.strip()
            # skip empty lines, [headers], and #comments
            if not line or line.startswith(("#")):
                continue
            if line.startswith("[") and line.endswith("]"):
                continue
            if "=" in line:
                name, value = line.split("=", 1)
                # clean up brackets, parentheses, and trailing comments
                clean_value = value.split("#")[0].strip().strip("[]()")
                try:
                    rgb = [int(x.strip()) for x in clean_value.split(',')]
                    imported_colors[name.strip()] = tuple(rgb)
                except ValueError:
                    # skip malformed lines gracefully
                    continue
    base_dict.update(imported_colors)
    return base_dict

# ======== BULB SCENES ========
_scene_list = []

def scene(func: callable) -> callable:            
    def wrapper(self, *args, **kwargs):     
        coro = func(self, *args, **kwargs)   
        self.run(coro)                                     
    if func.__name__ not in _scene_list:     
        _scene_list.append(func.__name__)    
    return wrapper                         

class Scenes:
    """
    Scenes run continuously (async), can incorporate changes over time and several colors
    e.g. flashes, fades
    complex colors that don't just use RGB, but also C/W values are also defined as (static) Scenes
    """
    scene_list = _scene_list  

    def __init__(self, bulb: 'Bulb', scene_runner: 'SceneRunner'):
        self.bulb = bulb
        self.scene_runner = scene_runner

    def run(self, scene_coroutine: callable):
        self.scene_runner.run(self.bulb, scene_coroutine)
        print(f"bulb {self.bulb.get_id_string()} scene set to {BOLD}{scene_coroutine.__name__}{RESET}")

    @scene
    async def reset(self):
        self.bulb.set_off()
        self.bulb.set_color(rgb=(255, 0, 0), brightness=100, cw=(0, 0))
        self.bulb.set_white(temp=6500, brightness=10)

    @scene
    async def rose(self):
        self.bulb.set_off()
        self.bulb.set_color(rgb=(255, 0, 65), cw=(0, 110))

    @scene
    async def blink_red_once(self):
        self.bulb.set_off()
        await asyncio.sleep(0.25)
        self.bulb.set_color((255, 0, 0), 100)
        await asyncio.sleep(0.75)
        self.bulb.set_off()

    @scene
    async def blink_red_loop(self):
        while True:
            self.bulb.set_on()
            self.bulb.set_color((255, 0, 0), 100)
            await asyncio.sleep(0.5)
            self.bulb.set_off()
            await asyncio.sleep(0.5)

    @scene
    async def fade_purple_pink(self):
        colors = [(164, 0, 255), (255, 0, 170), (255, 0, 92), (255, 0, 170)]
        steps = 50          
        delay = 0.30        
        while True:
            for i in range(len(colors)):
                r1_val, g1_val, b1_val = colors[i]
                r2_val, g2_val, b2_val = colors[(i + 1) % len(colors)]
                for step in range(steps):
                    t = step / steps                       
                    r_val = int(r1_val + (r2_val - r1_val) * t)              
                    g_val = int(g1_val + (g2_val - g1_val) * t)
                    b_val = int(b1_val + (b2_val - b1_val) * t)
                    self.bulb.set_color(rgb=(r_val, g_val, b_val))
                    await asyncio.sleep(delay)

    @scene
    async def fade_rgb(self, color1: tuple[int, int, int], color2: tuple[int, int, int], steps: int = 50, delay: float = 0.30):
        """generic two color fade scene, using RGB values"""
        r1_val, g1_val, b1_val = color1
        r2_val, g2_val, b2_val = color2
        while True:
            # fade from color1 to color2
            for step in range(steps + 1):
                t = step / steps
                r_val = int(r1_val + (r2_val - r1_val) * t)
                g_val = int(g1_val + (g2_val - g1_val) * t)
                b_val = int(b1_val + (b2_val - b1_val) * t)
                self.bulb.set_color(rgb=(r_val, g_val, b_val))
                await asyncio.sleep(delay)
            # fade back from color2 to color1
            for step in range(steps + 1):
                t = step / steps
                r_val = int(r2_val + (r1_val - r2_val) * t)
                g_val = int(g2_val + (g1_val - g2_val) * t)
                b_val = int(b2_val + (b1_val - b2_val) * t)
                self.bulb.set_color(rgb=(r_val, g_val, b_val))
                await asyncio.sleep(delay)

    @scene
    async def fade_color(self, color_name1: str, color_name2: str, steps: int = 50, delay: float = 0.30):
        """generic two color fade scene, using color_list names"""
        if color_name1 not in color_list or color_name2 not in color_list:
            raise ValueError(f"One or both color names not found in color_list: '{color_name1}', '{color_name2}'")
        r1_val, g1_val, b1_val = color_list[color_name1]
        r2_val, g2_val, b2_val = color_list[color_name2]
        while True:
            # fade from color1 to color2
            for step in range(steps + 1):
                t = step / steps
                r_val = int(r1_val + (r2_val - r1_val) * t)
                g_val = int(g1_val + (g2_val - g1_val) * t)
                b_val = int(b1_val + (b2_val - b1_val) * t)
                self.bulb.set_color(rgb=(r_val, g_val, b_val))
                await asyncio.sleep(delay)
            # fade back from color2 to color1
            for step in range(steps + 1):
                t = step / steps
                r_val = int(r2_val + (r1_val - r2_val) * t)
                g_val = int(g2_val + (g1_val - g2_val) * t)
                b_val = int(b2_val + (b1_val - b2_val) * t)
                self.bulb.set_color(rgb=(r_val, g_val, b_val))
                await asyncio.sleep(delay)



# ======== BACKEND ========
def ip_to_int(ip: str) -> int:
    """helper function to convert an IP str to int for numerical sorting"""
    return struct.unpack("!I", socket.inet_aton(ip))[0]

def ping_bulb(ip: str, timeout_sec: int = 12) -> bool:
    """check if a bulb IP responds to a UNIX system ping"""
    command = ['ping', '-c', '1', '-W', str(timeout_sec), ip]
    try:
        return subprocess.run(command, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0
    except:
        return False

def discover_bulbs(timeout_sec: int = 2) -> list[str]:
    """broadcast discovery and return list of responding IPs, sorted numerically"""
    msg = json.dumps({"method": "registration", "params": {
        "phoneMac": "AAAAAAAAAAAA",    
        "register": True,
        "phoneIp": "000.000.000.000",  
        "id": "1"
    }}).encode()
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_BROADCAST, 1)
    sock.settimeout(timeout_sec)
    sock.sendto(msg, ("255.255.255.255", 38899))
    found = []
    try:
        while True:
            data, (ip, port) = sock.recvfrom(1024)
            if ip not in found:
                found.append(ip)
    except socket.timeout:
        pass
    finally:
        sock.close()
    return sorted(found, key=ip_to_int) # sort IPs numerically

def load_bulbs_from_file(manager: "BulbManager", file_path: str):
    """import bulb IPs from a text or ini file (one IP per line, ignoring comments) and add them"""
    if not file_path.lower().endswith((".bulbs", ".ips", ".ini", ".txt")):
        print(f"{ERROR}{FG_RED}unsupported file type{RESET}")
        return
    if not os.path.exists(file_path):
        print(f"{ERROR}{FG_RED}bulb file {file_path} not found.{RESET}")
        return
    try:
        with open(file_path, 'r', encoding='utf-8') as f:
            ips = []
            for line in f:
                ip = line.strip()
                if ip and not ip.startswith("#") and ip not in ips:
                    ips.append(ip)
        # sort imported IPs numerically so IDs match network order logic
        sorted_ips = sorted(ips, key=ip_to_int)
        for ip in sorted_ips:
            try:
                if ping_bulb(ip) == True:
                    manager.add_bulb(ip)
                else:
                    print(f"{ERROR}{FG_RED}bulb IP '{ip}' did not respond to ping{RESET}")
            except Exception as e:
                print(f"{ERROR}{FG_RED}could not add bulb IP '{ip}': {e}{RESET}")
    except Exception as e:
        print(f"{ERROR}{FG_RED}could not import bulbs from file {file_path}: {e}{RESET}") 

class UDPHandler:
    def __init__(self, interval_ms=100):
        self.commands = {}                  
        self.interval = interval_ms / 1000  
        self._lock = threading.Lock()        
        threading.Thread(target=self._worker, daemon=True).start()

    def _worker(self):
        while True:
            time.sleep(self.interval)
            with self._lock:
                if not self.commands:
                    continue  
                pending_items = list(self.commands.items())
                self.commands.clear()
            for (ip, port, method), msg in pending_items:
                self._send_udp(ip, port, msg)
            self.commands.clear()            

    @staticmethod
    def _send_udp(ip, port, msg):
        data = json.dumps(msg).encode()
        try:
            sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
            sock.sendto(data, (ip, port))
            sock.close()
        except Exception as e:
            print(f"{ERROR}{FG_RED}in UDPHandler: {e}{RESET}")

    def add_command(self, ip, port, msg):
        method = msg.get("method")
        self.commands[(ip, port, method)] = msg

class SceneRunner:
    def __init__(self):
        self.loop = asyncio.new_event_loop()
        self.current_tasks = {}  
        threading.Thread(target=self._start_loop, daemon=True).start()  

    def _start_loop(self):
        asyncio.set_event_loop(self.loop)
        self.loop.run_forever()

    def run(self, bulb: "Bulb", scene_coroutine: callable):
        bulb_id = bulb.get_id()
        if bulb_id in self.current_tasks:
            task = self.current_tasks[bulb_id]
            if not task.done():
                task.cancel()
        def _schedule():
            task = self.loop.create_task(scene_coroutine)
            self.current_tasks[bulb_id] = task
        self.loop.call_soon_threadsafe(_schedule)

    def cancel(self, bulb: "Bulb"):
        bulb_id = bulb.get_id()
        if bulb_id in self.current_tasks:
            task = self.current_tasks[bulb_id]
            if not task.done():
                task.cancel()

class Bulb:
    def __init__(self, id: int, ip: str, udp_handler: UDPHandler, scene_runner: SceneRunner):
        self.id = id                                      
        self.ip = ip                                      
        self.port = 38899                                 
        self.udp_handler = udp_handler                    
        self.scenes = Scenes(self, scene_runner)  

        print(f"{FG_CYAN}bulb registered:{RESET} id = {self.get_id_string()}, ip = {FG_CYAN}{self.ip}{RESET}")

        self.state = {
            "on": False,         
            "r": 255,            
            "g": 255,            
            "b": 255,            
            "c": 0,              
            "w": 0,              
            "brightness": 10,    
            "temp": 6500,        
            "mode": 'white'      
        }
        # initialize the bulb:
        self.set_color(rgb=(255, 255, 255), brightness=(10), cw=(255, 0))
        self.set_off()

    def get_id(self) -> int: return self.id
    def get_id_string(self) -> str: return f"[{BOLD}{self.id}{RESET}]"
    def get_ip(self) -> str: return self.ip
    def get_state(self) -> dict: return self.state

    def _update_state(self, **kwargs):
        self.state.update(kwargs)

    def set_on(self):
        msg = {"method": "setState", "params": {"state": True}}
        self.udp_handler.add_command(self.ip, self.port, msg)  
        self._update_state(on=True)                            

    def set_off(self): 
        msg = {"method": "setState", "params": {"state": False}}
        self.udp_handler.add_command(self.ip, self.port, msg)  
        self._update_state(on=False)                           

    def set_toggle(self):
        self.set_off() if self.state["on"] else self.set_on()
    
    def set_brightness(self, brightness: int):
        brightness = max(10, min(100, brightness))
        msg = {
            "method": "setPilot",
            "params": {"dimming": brightness}
        }
        self.udp_handler.add_command(self.ip, self.port, msg)
        self._update_state(brightness=brightness, on=True)
        
    def set_color(self, rgb: tuple[int, int, int], brightness: int = None, cw: tuple[int, int] = (0, 0)):
        if brightness is None:
            brightness = self.state['brightness']
        msg = {                                                
            "method": "setPilot",
            "params": {
                "r": rgb[0], "g": rgb[1], "b": rgb[2],
                "c": cw[0], "w": cw[1], "dimming": brightness
            }
        }
        self.udp_handler.add_command(self.ip, self.port, msg)  
        self._update_state(r=rgb[0], g=rgb[1], b=rgb[2], c=cw[0], w=cw[1], brightness=brightness, mode='rgb', on=True)

    def set_white(self, temp: int, brightness: int = None):
        if brightness is None:
            brightness = self.state['brightness']
        msg = {                                                
            "method": "setPilot",
            "params": {"temp": temp, "dimming": brightness}
        }
        self.udp_handler.add_command(self.ip, self.port, msg)  
        self._update_state(r=255, g=255, b=255, temp=temp, brightness=brightness, mode='white', on=True)

    def cancel_scene(self): self.scenes.scene_runner.cancel(self)

    def fetch_pilot(self, timeout_sec: int = 2) -> dict | None:
        """sends getPilot to query the exact live state from the physical bulb"""
        msg = json.dumps({"method": "getPilot"}).encode()
        sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        sock.settimeout(timeout_sec)
        try:
            sock.bind(("", 0))  # bind to ephemeral local port
            sock.sendto(msg, (self.ip, self.port))
            data, _ = sock.recvfrom(1024)
            res = json.loads(data.decode())
            return res.get("result", None)
        except Exception:
            return None
        finally:
            try:
                sock.close()
            except Exception:
                pass

class BulbManager:
    def __init__(self, udp_handler: UDPHandler, scene_runner: SceneRunner):
        self.bulbs: list[Bulb] = []
        self.udp_handler = udp_handler
        self.scene_runner = scene_runner

    def get_bulbs(self) -> list[Bulb]: return self.bulbs

    def get_bulb_by_id(self, id: int) -> Bulb:
        for bulb in self.bulbs:
            if bulb.get_id() == id:
                return bulb
        raise ValueError(f"ID {id} not found in Bulb list")

    def _get_next_id(self) -> int:
        used_ids = {bulb.get_id() for bulb in self.bulbs}
        for i in range(100):  
            if i not in used_ids:
                return i
        raise ValueError("Bulb IDs (0-99) exhausted")

    def add_bulb(self, ip: str) -> Bulb:
        for bulb in self.bulbs:
            if bulb.get_ip() == ip:
                raise ValueError(f"IP {ip} already used")
        new_id = self._get_next_id()
        new_bulb = Bulb(new_id, ip, self.udp_handler, self.scene_runner)
        self.bulbs.append(new_bulb)
        return new_bulb

    def rem_bulb(self, ip: str):
        for bulb in self.bulbs:
            if bulb.get_ip() == ip:
                self.bulbs.remove(bulb)
                return True
        raise ValueError(f"IP {ip} not found in Bulb list")

    def discover_bulbs_and_add(self):
        print(f"{BOLD}{FG_CYAN}discovering bulbs on local network...\n{RESET}")
        for bulb in self.bulbs.copy():  
            self.rem_bulb(ip=bulb.get_ip())
        # discover and assign IDs sequentially starting from 1 based on sorted IPs
        sorted_ips = discover_bulbs()
        for idx, ip in enumerate(sorted_ips, start=1):
            new_bulb = Bulb(idx, ip, self.udp_handler, self.scene_runner)
            self.bulbs.append(new_bulb)



# ======== CLI MAIN ========
def execute_command(manager: BulbManager, clean_input: list[str]) -> bool:
    """executes cmds -> returns True if program should terminate"""
    if not clean_input: 
        return False
    cmd = clean_input[0]

    try:
        if cmd in ("quit", "exit", "q"):
            terminate()
            return True
        
        elif cmd in ("list", "ls", "l"):
            print("\ndiscovered bulbs:")
            bulbs = manager.get_bulbs()
            if not bulbs:
                print(f"{ERROR}{FG_RED}no bulbs discovered.{RESET}")
            for bulb in bulbs:
                is_on = bulb.get_state()["on"]
                state_str = f"{FG_BRIGHT_GREEN}ON{RESET}" if is_on else f"{FG_RED}OFF{RESET}"
                print(f"  {bulb.get_id_string()} ip = {FG_CYAN}{bulb.get_ip()}{RESET} status = {state_str}")
            print()

        elif cmd == "on" and len(clean_input) == 2:
            bulb = manager.get_bulb_by_id(int(clean_input[1]))
            bulb.set_on()
            print(f"bulb {bulb.get_id_string()} turned {FG_BRIGHT_GREEN}ON{RESET}.")
        
        elif cmd == "off" and len(clean_input) == 2:
            bulb = manager.get_bulb_by_id(int(clean_input[1]))
            bulb.set_off()
            print(f"bulb {bulb.get_id_string()} turned {FG_RED}OFF{RESET}.")

        elif cmd in ("brightness", "bright", "b") and len(clean_input) == 3:
            bulb = manager.get_bulb_by_id(int(clean_input[1]))
            level = int(clean_input[2])
            bulb.cancel_scene()
            bulb.set_brightness(level)
            print(f"bulb {bulb.get_id_string()} brightness set to {BOLD}{bulb.get_state()['brightness']}%{RESET}.")
        
        elif cmd in ("color", "c") and len(clean_input) == 3:
            bulb = manager.get_bulb_by_id(int(clean_input[1]))
            color_name = clean_input[2]
            if color_name in color_list:
                rgb = color_list[color_name]
                bulb.cancel_scene()
                bulb.set_color(rgb=rgb)
                print(f"bulb {bulb.get_id_string()} color set to {BOLD}{color_name}{RESET}")
            else:
                colors_formatted = ", ".join([f"{BOLD}{c}{RESET}" for c in color_list.keys()])
                print(f"{ERROR}{FG_RED}unknown color{RESET}\navailable colors: {colors_formatted}")
        
        elif cmd == "rgb" and len(clean_input) == 3:
            bulb = manager.get_bulb_by_id(int(clean_input[1]))
            try:
                r_val, g_val, b_val = map(int, clean_input[2].split(","))
                if all(0 <= val <= 255 for val in (r_val, g_val, b_val)):
                    bulb.cancel_scene()
                    bulb.set_color(rgb=(r_val, g_val, b_val))
                    print(f"bulb {bulb.get_id_string()} color set to RGB ({BOLD}{r_val}, {g_val}, {b_val}{RESET})")
                else:
                    print(f"{ERROR}{FG_RED}RGB values must be between 0 and 255{RESET}")
            except (ValueError, IndexError):
                print(f"{ERROR}{FG_RED}invalid RGB format. Use: rgb <id> <R,G,B> (e.g. rgb 0 255,128,0){RESET}")

        elif cmd in ("temp", "t") and len(clean_input) == 3:
            bulb = manager.get_bulb_by_id(int(clean_input[1]))
            kelvin = int(clean_input[2])
            min_k, max_k = 2200, 6500
            if min_k <= kelvin <= max_k:
                bulb.cancel_scene()
                ratio = (kelvin - min_k) / (max_k - min_k)
                c_val = int(ratio * 255)
                w_val = int((1 - ratio) * 255)
                bulb.set_color(rgb=(0, 0, 0), cw=(c_val, w_val))
                print(f"bulb {bulb.get_id_string()} temp set to {BOLD}{kelvin}K{RESET} (C={c_val}, W={w_val})")
            else:
                print(f"{ERROR}{FG_RED}temp must be between {min_k}K and {max_k}K.{RESET}")

        elif cmd in ("scene", "s") and len(clean_input) == 3:
            bulb = manager.get_bulb_by_id(int(clean_input[1]))
            s_name = clean_input[2]
            if s_name in Scenes.scene_list:
                getattr(bulb.scenes, s_name)()
            else:
                scenes_formatted = ", ".join([f"{BOLD}{s}{RESET}" for s in Scenes.scene_list])
                print(f"{ERROR}{FG_RED}unknown scene{RESET}\navailable scenes: {scenes_formatted}")

        elif cmd in ("fade_rgb", "f_rgb", "frgb") and len(clean_input) == 4:
            bulb = manager.get_bulb_by_id(int(clean_input[1]))
            try:
                r1, g1, b1 = map(int, clean_input[2].split(","))
                r2, g2, b2 = map(int, clean_input[3].split(","))
                if all(0 <= val <= 255 for val in (r1, g1, b1, r2, g2, b2)):
                    bulb.scenes.fade_rgb((r1, g1, b1), (r2, g2, b2))
                    print(f"bulb {bulb.get_id_string()} scene set to {BOLD}fade_rgb ({r1},{g1},{b1}) ({r2},{g2},{b2}){RESET}")
                else:
                    print(f"{ERROR}{FG_RED}RGB values must be between 0 and 255{RESET}")
            except (ValueError, IndexError):
                print(f"{ERROR}{FG_RED}invalid fade_rgb format. use: fade-rgb <id> <R1,G1,B1> <R2,G2,B2>{RESET}")

        elif cmd in ("fade_color", "fade_c", "f_c", "fc") and len(clean_input) == 4:
            bulb = manager.get_bulb_by_id(int(clean_input[1]))
            c1, c2 = clean_input[2], clean_input[3]
            if c1 in color_list and c2 in color_list:
                bulb.scenes.fade_color(c1, c2)
                print(f"bulb {bulb.get_id_string()} scene set to {BOLD}fade_color ({c1}) ({c2}){RESET}")
            else:
                colors_formatted = ", ".join([f"{BOLD}{c}{RESET}" for c in color_list.keys()])
                print(f"{ERROR}{FG_RED}unknown color name(s){RESET}\navailable colors: {colors_formatted}")

        elif cmd in ("reset", "r") and len(clean_input) == 2:
            bulb = manager.get_bulb_by_id(int(clean_input[1]))
            getattr(bulb.scenes, "reset")()

        elif cmd in ("poll", "p") and len(clean_input) == 2:
            bulb = manager.get_bulb_by_id(int(clean_input[1]))
            print(f"polling bulb {bulb.get_id_string()} at {bulb.get_ip()}...")
            pilot = bulb.fetch_pilot()
            if pilot:
                print(f"{FG_CYAN}response:{RESET}")
                print(json.dumps(pilot, indent=2))
            else:
                print(f"{ERROR}{FG_RED}no response received from bulb (timeout){RESET}")

        elif cmd in ("color_list", "color_l", "c_l", "cl"):
            colors_formatted = ", ".join([f"{BOLD}{c}{RESET}" for c in color_list.keys()])
            print(f"\navailable colors: {colors_formatted}")

        elif cmd in ("scene_list", "scene_l", "s_l", "sl"):
            scenes_formatted = ", ".join([f"{BOLD}{s}{RESET}" for s in Scenes.scene_list])
            print(f"\navailable scenes: {scenes_formatted}")

        elif cmd in ("load_color_file", "l_c_f", "lcf") and len(clean_input) == 2:
            file_path = clean_input[1]
            try:
                load_colors_from_file(file_path)
                print(f"colors loaded from file: {BOLD}{file_path}{RESET}")
            except Exception as e:
                print(f"{ERROR}{FG_RED}could not load colors from file \"{file_path}\": {e}")

        else:
            print(f"{ERROR}{FG_RED}invalid command or arguments.{RESET}")

    except Exception as e:
        print(f"{ERROR}{FG_RED}executing command: {e}{RESET}")
    
    return False


def print_help():
    print(f"\n\n{BOLD}{FG_WHITE}{BG_BLUE} *** WiZ-CLI Control *** {RESET}")
    print(f"{FG_YELLOW}-------------------------------------------------------------------------------{RESET}")
    print("available commands:")
    print(f"  {FG_CYAN}list{RESET}                        - list discovered bulbs")
    print(f"  {FG_CYAN}on{RESET} <id>                     - turn bulb ON")
    print(f"  {FG_CYAN}off{RESET} <id>                    - turn bulb OFF")
    print(f"  {FG_CYAN}brightness{RESET} <id> <10-100>    - set bulb brightness percentage")
    print(f"  {FG_CYAN}color{RESET} <id> <color_name>     - set bulb to a color (e.g. red, green)")
    print(f"  {FG_CYAN}rgb{RESET} <id> <R,G,B>            - set bulb RGB value (e.g. rgb 0 255 128 0)")
    print(f"  {FG_CYAN}temp{RESET} <id> <2200-6500>       - set bulb to white, temparature in Kelvin")
    print(f"  {FG_CYAN}scene{RESET} <id> <scene_name>     - run scene on bulb")
    print(f"  {FG_CYAN}fade_rgb{RESET} <id> <R1,G1,B1> <R2,G2,B2>    - fade between two RGB values")
    print(f"  {FG_CYAN}fade_color{RESET} <id> <color1> <color2>      - fade between two named colors")
    print(f"  {FG_CYAN}reset{RESET} <id>                  - run reset scene on bulb")
    print(f"  {FG_CYAN}poll{RESET} <id>                   - poll bulb for physical device state")
    print(f"  {FG_CYAN}color_list{RESET}                  - list available bulb colors")
    print(f"  {FG_CYAN}scene_list{RESET}                  - list available bulb scenes")
    print(f"  {FG_CYAN}load_color_file{RESET} <path>      - load colors from file (.toml/.colors/.ini/.txt)")
    print(f"  {FG_CYAN}quit{RESET}                        - exit the program")
    print(f"{FG_GREY}note: commands can be abbreviated{RESET}")
    print(f"{FG_YELLOW}-------------------------------------------------------------------------------{RESET}")


def run_cli(manager: BulbManager):
    os.system("clear")  # clear terminal (UNIX)
    print(f"{BOLD}{UNDERLINE}WiZ-CLI - PROGRAM START{RESET}")
    print(f"version: {__version__}\n")
    print_help()

    while True:
        try:
            prompt = f"\n{BOLD}{FG_WHITE}{BG_BLUE}wiz-cli:{RESET} "
            raw_input = input(prompt).strip()
            clean_input = raw_input.lower().split()
            if not clean_input: 
                continue
            if execute_command(manager, clean_input):
                break
        except (KeyboardInterrupt, EOFError):
            terminate()
            break


def terminate(signal=None, frame=None, silent=False, *args):
    """handle end of program as well as Ctrl+C gracefully"""
    if signal:   # --> exit via Ctrl-C (signal arg is passed)
        print()  # extra newline to match "quit" cmd cursor position
    if not silent:
        print(f"\n{BOLD}{UNDERLINE}PROGRAM END{RESET}\n\n\n")
    sys.exit(0)


def main():
    signal.signal(signal.SIGINT, terminate)  # capture Ctrl+C -> call terminate(signal, frame)
    # print("\033[8;30;80t", end="")  # force terminal wndw to 30 rows by 80 columns

    # -- check if help flag was passed --> print help and exit (ignore additional args)
    if len(sys.argv) >= 2 and sys.argv[1].lower() in ("-h", "--help"):
        print_help()
        terminate(silent=True)  # terminate silently

    # -- check if version flag was passed --> print version and exit  (ignore additional args)
    if len(sys.argv) >= 2 and sys.argv[1].lower() in ("-v", "--version"):
        print(f"version: {BOLD}{__version__}{RESET}\n")
        terminate(silent=True)  # terminate silently

    
    udp_handler = UDPHandler()
    scene_runner = SceneRunner()
    manager = BulbManager(udp_handler, scene_runner)
    
    # populate bulbs
    # -- check if bulbs file flag was passed
    if len(sys.argv) >= 3 and sys.argv[1].lower() in ("-i", "--import-bulbs"):
        file_path = sys.argv[2]
        print(f"{BOLD}{FG_CYAN}loading bulbs from file: {BOLD}{file_path}{RESET}\n")
        load_bulbs_from_file(manager, file_path)
        time.sleep(0.5)
        cmd_args = sys.argv[3:]  # shift arguments to skip the file flag and path
    # -- discover bulbs on local network
    else:
        manager.discover_bulbs_and_add()
        time.sleep(0.5)  # wait to let UDP broadcast responses populate
        cmd_args = sys.argv[1:]

    # set execution mode
    # -- check if command line args were given
    if cmd_args:
        args_input = [arg.lower() for arg in cmd_args]
        print(f"passed args: {BOLD}{args_input}{RESET}\n")
        execute_command(manager, args_input)
        time.sleep(0.2)  
        terminate()
    # -- run interactive REPL
    else:
        run_cli(manager)


if __name__ == '__main__':
    main()