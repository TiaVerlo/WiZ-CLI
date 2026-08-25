package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// GLOBAL_SUBLIME_SAVECOUNT = true  # sublime flag: auto-increase version
const version = "6.1-daemon"

// ===== ANSI COLOR CODES =========================================================================
const (
	Reset      = "\033[0m"
	Bold       = "\033[1m"
	Underline  = "\033[4m"
	FgBlack    = "\033[30m"
	FgRed      = "\033[31m"
	FgGreen    = "\033[32m"
	FgYellow   = "\033[33m"
	FgBlue     = "\033[34m"
	FgMagenta  = "\033[35m"
	FgCyan     = "\033[36m"
	FgWhite    = "\033[37m"
	FgBrtCyan  = "\033[96m"
	FgBrtGreen = "\033[92m"
	FgGrey     = "\033[90m"
	BgBlue     = "\033[44m"
	BgGrey     = "\033[100m"
)

var ErrorPrefix = fmt.Sprintf("%s%sERROR: %s", Bold, FgRed, Reset)

// ===== DATA STRUCTURES ==========================================================================
type RGB struct{ R, G, B int }
type CW struct{ C, W int }

var colorList = map[string]RGB{
	"red_pink":     {255, 0, 90},
	"blue_pink":    {255, 0, 170},
	"blue_purple":  {95, 0, 255},
	"purple":       {165, 0, 255},
	"yellow_green": {150, 255, 0},
	"green":        {30, 255, 0},
	"red":          {255, 0, 0},
	"yellow":       {255, 255, 0},
	"blue":         {0, 0, 255},
	"orange":       {255, 128, 0},
	"deep_orange":  {255, 69, 0},
	"amber":        {255, 191, 0},
	"gold":         {255, 215, 0},
	"turquoise":    {64, 224, 208},
	"emerald":      {80, 200, 120},
	"teal":         {0, 128, 128},
	"cyan":         {0, 255, 255},
	"lightblue":    {135, 206, 235},
	"lime":         {0, 255, 0},
	"forest_green": {34, 139, 34},
	"mint":         {152, 251, 152},
	"sky_blue":     {0, 191, 255},
	"navy":         {0, 0, 128},
	"indigo":       {75, 0, 130},
	"magenta":      {255, 0, 255},
	"orchid":       {218, 112, 214},
	"bright_pink":  {255, 105, 180},
	"pink":         {255, 20, 147},
	"violet":       {238, 130, 238},
	"lavender":     {230, 230, 250},
	"plum":         {221, 160, 221},
	"coral":        {255, 127, 80},
}

func loadColorsFromFile(filePath string) error {
	ext := strings.ToLower(filePath)
	if !strings.HasSuffix(ext, ".toml") && !strings.HasSuffix(ext, ".colors") {
		fmt.Printf("%s%sunsupported file type%s\n", ErrorPrefix, FgRed, Reset)
		return fmt.Errorf("unsupported file type")
	}
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("%s%scould not open file: %v%s\n", ErrorPrefix, FgRed, err, Reset)
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	inColorsSection := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if strings.ToUpper(line) == "[COLORS]" {
				inColorsSection = true
			} else {
				inColorsSection = false
			}
			continue
		}

		if !inColorsSection {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			name := strings.TrimSpace(parts[0])
			valRaw := strings.Split(parts[1], "#")[0]
			valRaw = strings.TrimFunc(valRaw, func(r rune) bool {
				return r == ' ' || r == '[' || r == ']' || r == '(' || r == ')'
			})
			rgbParts := strings.Split(valRaw, ",")
			if len(rgbParts) == 3 {
				r, errR := strconv.Atoi(strings.TrimSpace(rgbParts[0]))
				g, errG := strconv.Atoi(strings.TrimSpace(rgbParts[1]))
				b, errB := strconv.Atoi(strings.TrimSpace(rgbParts[2]))
				if errR == nil && errG == nil && errB == nil {
					colorList[name] = RGB{r, g, b}
				}
			}
		}
	}
	return nil
}

// ===== SCENE STRUCTS & SCENESCRIPT PARSER =======================================================
type Action struct {
	Method     string
	R, G, B    int
	Brightness int
	C, W       int
	Temp       int
	Ms         int
	Steps      int
	DelayMs    int
	Colors     []RGB
}

type SceneScript struct {
	Description string
	Loop        bool
	Actions     []Action
}

var loadedScenes = make(map[string]SceneScript)

func loadScenesFromFile(filePath string) error {
	ext := strings.ToLower(filePath)
	if !strings.HasSuffix(ext, ".toml") && !strings.HasSuffix(ext, ".scenes") {
		fmt.Printf("%s%sunsupported file type%s\n", ErrorPrefix, FgRed, Reset)
		return fmt.Errorf("unsupported file type")
	}

	file, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("%s%scould not open file: %v%s\n", ErrorPrefix, FgRed, err, Reset)
		return err
	}
	defer file.Close()

	loadedScenes = make(map[string]SceneScript)
	scanner := bufio.NewScanner(file)

	var activeScene string
	var currentDef SceneScript
	inScenes, inActions := false, false

	var actionBuffer string
	var braceCount int

	saveScene := func() {
		if activeScene != "" {
			loadedScenes[activeScene] = currentDef
		}
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			lowerLine := strings.ToLower(line)
			if lowerLine == "[scenes]" {
				inScenes = true
			} else if strings.HasPrefix(lowerLine, "[scenes.") {
				inScenes = true
				saveScene()
				activeScene = strings.TrimSpace(line[8 : len(line)-1])
				currentDef = SceneScript{}
				inActions = false
				actionBuffer = ""
				braceCount = 0
			} else {
				inScenes = false
			}
			continue
		}

		if !inScenes {
			continue
		}

		if inActions {
			if line == "]" && braceCount == 0 {
				inActions = false
				continue
			}

			for _, ch := range line {
				if ch == '{' {
					braceCount++
				} else if ch == '}' {
					braceCount--
				}
			}

			actionBuffer += " " + line

			if braceCount == 0 && strings.Contains(actionBuffer, "{") {
				if action := parseActionInline(actionBuffer); action.Method != "" {
					currentDef.Actions = append(currentDef.Actions, action)
				}
				actionBuffer = ""
			}
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(strings.ToLower(parts[0]))
			val := strings.TrimSpace(parts[1])

			switch key {
			case "description":
				currentDef.Description = strings.Trim(val, "\"'")
			case "loop":
				currentDef.Loop = (val == "true")
			case "actions":
				inActions = true
			}
		}
	}

	saveScene()
	return nil
}

func parseActionInline(block string) Action {
	act := Action{Brightness: -1}

	reMethod := regexp.MustCompile(`\bmethod\s*=\s*["']([^"']+)["']`)
	if m := reMethod.FindStringSubmatch(block); len(m) > 1 {
		act.Method = m[1]
	}

	act.Brightness = extractValRegex(block, "brightness", -1)
	act.Temp = extractValRegex(block, "temp", 0)
	act.Ms = extractValRegex(block, "ms", 0)
	act.Steps = extractValRegex(block, "steps", 0)
	act.DelayMs = extractValRegex(block, "delay_ms", 0)

	act.R = extractValRegex(block, "r", 0)
	act.G = extractValRegex(block, "g", 0)
	act.B = extractValRegex(block, "b", 0)

	act.C = extractValRegex(block, "c", 0)
	act.W = extractValRegex(block, "w", 0)

	if strings.Contains(block, "colors") {
		act.Colors = extractColorListRegex(block)
	}

	return act
}

func extractValRegex(block, key string, defaultVal int) int {
	re := regexp.MustCompile(`\b` + key + `\s*=\s*(-?\d+)`)
	if matches := re.FindStringSubmatch(block); len(matches) > 1 {
		if v, err := strconv.Atoi(matches[1]); err == nil {
			return v
		}
	}
	return defaultVal
}

func extractColorListRegex(block string) []RGB {
	var colors []RGB
	idx := strings.Index(block, "colors")
	if idx == -1 {
		return colors
	}
	sub := block[idx:]
	parts := strings.Split(sub, "{")

	for _, p := range parts {
		if strings.Contains(p, "r") && strings.Contains(p, "g") && strings.Contains(p, "b") {
			r := extractValRegex(p, "r", 0)
			g := extractValRegex(p, "g", 0)
			b := extractValRegex(p, "b", 0)
			colors = append(colors, RGB{r, g, b})
		}
	}
	return colors
}

// ===== SCENE RUNNER =============================================================================
func (b *Bulb) cancelScene() {
	b.SceneRunner.cancel(b.ID)
}

func (b *Bulb) runScene(name string, args ...string) error {
	var sceneFunc func(context.Context)
	nameLower := strings.ToLower(name)

	if nameLower == "reset" {
		sceneFunc = func(ctx context.Context) {
			b.setWhite(6500, 100)
		}
	} else if nameLower == "fade_rgb" {
		if len(args) < 2 || len(args) > 3 {
			return fmt.Errorf("fade_rgb requires 2 RGB arguments and an optional cycle length in seconds")
		}
		c1, err1 := parseRGB(args[0])
		c2, err2 := parseRGB(args[1])
		if err1 != nil || err2 != nil {
			return fmt.Errorf("invalid RGB format")
		}
		steps, delay := 50, 300*time.Millisecond
		if len(args) == 3 {
			cycleSec, err := strconv.ParseFloat(args[2], 64)
			if err != nil || cycleSec <= 0 {
				return fmt.Errorf("invalid cycle length: %s", args[2])
			}
			delay = time.Duration((cycleSec * float64(time.Second)) / float64(2*steps))
		}
		sceneFunc = func(ctx context.Context) {
			for {
				if !fadeTransition(ctx, b, c1, c2, steps, delay) {
					return
				}
				if !fadeTransition(ctx, b, c2, c1, steps, delay) {
					return
				}
			}
		}
	} else if nameLower == "fade_color" {
		if len(args) < 2 || len(args) > 3 {
			return fmt.Errorf("fade_color requires 2 color arguments and an optional cycle length in seconds")
		}
		c1, ok1 := colorList[args[0]]
		c2, ok2 := colorList[args[1]]
		if !ok1 || !ok2 {
			return fmt.Errorf("unknown color name(s)")
		}
		steps, delay := 50, 300*time.Millisecond
		if len(args) == 3 {
			cycleSec, err := strconv.ParseFloat(args[2], 64)
			if err != nil || cycleSec <= 0 {
				return fmt.Errorf("invalid cycle length: %s", args[2])
			}
			delay = time.Duration((cycleSec * float64(time.Second)) / float64(2*steps))
		}
		sceneFunc = func(ctx context.Context) {
			for {
				if !fadeTransition(ctx, b, c1, c2, steps, delay) {
					return
				}
				if !fadeTransition(ctx, b, c2, c1, steps, delay) {
					return
				}
			}
		}
	} else if sceneDef, ok := loadedScenes[nameLower]; ok {
		sceneFunc = func(ctx context.Context) {
			executeSceneScript(ctx, b, sceneDef)
		}
	} else {
		return fmt.Errorf("unknown scene: %s", name)
	}

	b.SceneRunner.run(b.ID, sceneFunc)
	fmt.Printf("bulb [%s%d%s] scene set to %s%s%s\n", Bold, b.ID, Reset, Bold, name, Reset)
	return nil
}

func executeSceneScript(ctx context.Context, b *Bulb, scene SceneScript) {
	for {
		for _, action := range scene.Actions {
			select {
			case <-ctx.Done():
				return
			default:
			}

			switch action.Method {
			case "setOff":
				b.setOff()
			case "setOn":
				b.setOn()
			case "setColor":
				rgb := RGB{action.R, action.G, action.B}
				cw := CW{action.C, action.W}
				b.setColor(rgb, action.Brightness, cw)
			case "setWhite":
				b.setWhite(action.Temp, action.Brightness)
			case "delay":
				ms := action.Ms
				if ms == 0 {
					ms = 100
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Duration(ms) * time.Millisecond):
				}
			case "fadeSequence":
				steps := action.Steps
				if steps == 0 {
					steps = 50
				}
				delayMs := action.DelayMs
				if delayMs == 0 {
					delayMs = 300
				}
				colors := action.Colors
				if len(colors) > 1 {
					for i := range colors {
						c1, c2 := colors[i], colors[(i+1)%len(colors)]
						if !fadeTransition(ctx, b, c1, c2, steps, time.Duration(delayMs)*time.Millisecond) {
							return
						}
					}
				}
			case "fadeTo":
				ms := action.Ms
				if ms == 0 {
					ms = 1000
				}
				steps := action.Steps
				if steps == 0 {
					steps = 30
				}
				stepDelay := time.Duration(ms/steps) * time.Millisecond
				if stepDelay < 10*time.Millisecond {
					stepDelay = 10 * time.Millisecond
					steps = ms / 10
					if steps < 1 {
						steps = 1
					}
				}
				rgb := RGB{action.R, action.G, action.B}
				bright := action.Brightness
				if !fadeTo(ctx, b, rgb, bright, steps, stepDelay) {
					return
				}
			}
		}

		if !scene.Loop {
			break
		}
	}
}

func fadeTransition(ctx context.Context, b *Bulb, c1, c2 RGB, steps int, delay time.Duration) bool {
	for step := 0; step <= steps; step++ {
		t := float64(step) / float64(steps)
		r := int(float64(c1.R) + float64(c2.R-c1.R)*t)
		g := int(float64(c1.G) + float64(c2.G-c1.G)*t)
		blue := int(float64(c1.B) + float64(c2.B-c1.B)*t)
		b.setColor(RGB{r, g, blue}, -1, CW{0, 0})
		select {
		case <-ctx.Done():
			return false
		case <-time.After(delay):
		}
	}
	return true
}

func fadeTo(ctx context.Context, b *Bulb, targetRGB RGB, targetBright int, steps int, stepDelay time.Duration) bool {
	b.mu.Lock()
	startRGB := RGB{R: b.State.R, G: b.State.G, B: b.State.B}
	startBright := b.State.Brightness
	if !b.State.On {
		startBright = 0
		startRGB = targetRGB
	}
	b.mu.Unlock()

	if targetBright < 0 {
		targetBright = startBright
	}

	for step := 1; step <= steps; step++ {
		t := float64(step) / float64(steps)
		r := int(float64(startRGB.R) + float64(targetRGB.R-startRGB.R)*t)
		g := int(float64(startRGB.G) + float64(targetRGB.G-startRGB.G)*t)
		blue := int(float64(startRGB.B) + float64(targetRGB.B-startRGB.B)*t)
		bright := int(float64(startBright) + float64(targetBright-startBright)*t)

		if bright <= 0 && targetBright == 0 {
			b.setOff()
		} else {
			if bright < 1 {
				bright = 1
			}
			b.setColor(RGB{r, g, blue}, bright, CW{0, 0})
		}

		select {
		case <-ctx.Done():
			return false
		case <-time.After(stepDelay):
		}
	}

	if targetBright == 0 {
		b.setOff()
	}
	return true
}

func parseRGB(in string) (RGB, error) {
	parts := strings.Split(in, ",")
	if len(parts) != 3 {
		return RGB{}, fmt.Errorf("bad format")
	}
	r, e1 := strconv.Atoi(parts[0])
	g, e2 := strconv.Atoi(parts[1])
	b, e3 := strconv.Atoi(parts[2])
	if e1 != nil || e2 != nil || e3 != nil || r < 0 || r > 255 || g < 0 || g > 255 || b < 0 || b > 255 {
		return RGB{}, fmt.Errorf("out of range")
	}
	return RGB{r, g, b}, nil
}

// ===== FZF AND MACOS / OSAS COLOR PICKERS =======================================================
func findFZFBinary() (string, error) {
	if path, err := exec.LookPath("fzf"); err == nil {
		return path, nil
	}
	commonPaths := []string{
		"/opt/homebrew/bin/fzf",
		"/usr/local/bin/fzf",
		"/usr/bin/fzf",
		filepath.Join(os.Getenv("HOME"), ".fzf", "bin", "fzf"),
	}
	for _, p := range commonPaths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("fzf binary not found in $PATH or standard locations (/opt/homebrew/bin, /usr/local/bin)")
}

func fzfPicker(items []string) (string, error) {
	if len(items) == 0 {
		return "", fmt.Errorf("no items available")
	}

	fzfBin, err := findFZFBinary()
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	for _, item := range items {
		sb.WriteString(item + "\n")
	}

	cmd := exec.Command(fzfBin)
	cmd.Env = append(os.Environ(), "FZF_DEFAULT_OPTS="+
		"--color=fg:white,fg+:white "+
		"--color=header:green "+
		"--color=info:green "+
		"--color=pointer:yellow "+
		"--color=scrollbar:gray "+
		"--pointer=\"→\" "+
		"--color=prompt:white "+
		"--prompt=\"...\" "+
		"--color=spinner:yellow "+
		"--layout=reverse --info=right",
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("failed to open stdin pipe: %w", err)
	}

	var outBytes bytes.Buffer
	cmd.Stdout = &outBytes
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start fzf (%s): %w", fzfBin, err)
	}

	go func() {
		defer stdin.Close()
		io.WriteString(stdin, sb.String())
	}()

	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 || exitErr.ExitCode() == 130 {
				return "", nil
			}
		}
		return "", fmt.Errorf("fzf execution failed: %w", err)
	}

	return strings.TrimSpace(outBytes.String()), nil
}

func fzfScenePicker() (string, error) {
	if len(loadedScenes) == 0 {
		return "", fmt.Errorf("no scenes loaded")
	}
	var scenes []string
	for name := range loadedScenes {
		scenes = append(scenes, name)
	}
	sort.Strings(scenes)
	return fzfPicker(scenes)
}

func fzfColorPicker() (string, error) {
	if len(colorList) == 0 {
		return "", fmt.Errorf("no colors loaded")
	}
	var colors []string
	for name := range colorList {
		colors = append(colors, name)
	}
	sort.Strings(colors)
	return fzfPicker(colors)
}

func macOSColorPicker() (RGB, error) {
	cmd := exec.Command("osascript", "-e", "choose color")
	out, err := cmd.Output()
	if err != nil {
		return RGB{}, err
	}

	raw := strings.TrimSpace(string(out))
	parts := strings.Split(raw, ",")
	if len(parts) != 3 {
		return RGB{}, fmt.Errorf("invalid color picker response format")
	}

	r16, e1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	g16, e2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	b16, e3 := strconv.Atoi(strings.TrimSpace(parts[2]))

	if e1 != nil || e2 != nil || e3 != nil {
		return RGB{}, fmt.Errorf("failed to parse color picker output")
	}

	return RGB{
		R: r16 / 257,
		G: g16 / 257,
		B: b16 / 257,
	}, nil
}

// ===== BACKEND ==================================================================================
type BulbState struct {
	On         bool
	R, G, B    int
	C, W       int
	Brightness int
	Temp       int
	Mode       string
}

type MsgPayload struct {
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params,omitempty"`
}

type UDPHandler struct {
	commands chan commandReq
}

type commandReq struct {
	ip   string
	port int
	msg  MsgPayload
}

func newUDPHandler(intervalMs int) *UDPHandler {
	handler := &UDPHandler{
		commands: make(chan commandReq, 100),
	}
	go handler.worker(time.Duration(intervalMs) * time.Millisecond)
	return handler
}

func (u *UDPHandler) worker(interval time.Duration) {
	ticker := time.NewTicker(interval)
	pending := make(map[string]commandReq)

	for {
		select {
		case req := <-u.commands:
			key := fmt.Sprintf("%s:%d:%s", req.ip, req.port, req.msg.Method)
			pending[key] = req
		case <-ticker.C:
			for _, req := range pending {
				u.sendUDP(req.ip, req.port, req.msg)
			}
			pending = make(map[string]commandReq)
		}
	}
}

func (u *UDPHandler) sendUDP(ip string, port int, msg MsgPayload) {
	data, _ := json.Marshal(msg)
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", ip, port))
	if err != nil {
		return
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.Write(data)
}

func (u *UDPHandler) addCommand(ip string, port int, msg MsgPayload) {
	u.commands <- commandReq{ip: ip, port: port, msg: msg}
}

type SceneRunner struct {
	mu    sync.Mutex
	tasks map[int]context.CancelFunc
}

func newSceneRunner() *SceneRunner {
	return &SceneRunner{tasks: make(map[int]context.CancelFunc)}
}

func (s *SceneRunner) run(bulbID int, sceneFunc func(context.Context)) {
	s.cancel(bulbID)
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.tasks[bulbID] = cancel
	s.mu.Unlock()
	go sceneFunc(ctx)
}

func (s *SceneRunner) cancel(bulbID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cancel, exists := s.tasks[bulbID]; exists {
		cancel()
		delete(s.tasks, bulbID)
	}
}

func (s *SceneRunner) cancelAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, cancel := range s.tasks {
		cancel()
		delete(s.tasks, id)
	}
}

func (s *SceneRunner) hasActiveTasks() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.tasks) > 0
}

func setupSignalHandler(cancel context.CancelFunc, sceneRunner *SceneRunner) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		sceneRunner.cancelAll()
		if cancel != nil {
			cancel()
		}
		fmt.Printf("\n%s%sPROGRAM END%s\n\n", Bold, Underline, Reset)
		os.Exit(0)
	}()
}

type Bulb struct {
	ID          int
	IP          string
	Port        int
	UDPHandler  *UDPHandler
	SceneRunner *SceneRunner
	State       BulbState
	mu          sync.Mutex
}

func NewBulb(id int, ip string, udp *UDPHandler, runner *SceneRunner) *Bulb {
	b := &Bulb{
		ID:          id,
		IP:          ip,
		Port:        38899,
		UDPHandler:  udp,
		SceneRunner: runner,
		State: BulbState{
			On: false,
			R:  255, G: 255, B: 255,
			C: 0, W: 0,
			Brightness: 10,
			Temp:       6500,
			Mode:       "white",
		},
	}
	fmt.Printf("%sbulb registered:%s id = [%s%d%s], ip = %s%s%s\n",
		FgCyan, Reset, Bold, b.ID, Reset, FgCyan, b.IP, Reset)

	return b
}

func (b *Bulb) setOn() {
	b.mu.Lock()
	b.State.On = true
	b.mu.Unlock()
	b.UDPHandler.addCommand(b.IP, b.Port, MsgPayload{Method: "setState", Params: map[string]interface{}{"state": true}})
}

func (b *Bulb) setOff() {
	b.mu.Lock()
	b.State.On = false
	b.mu.Unlock()
	b.UDPHandler.addCommand(b.IP, b.Port, MsgPayload{Method: "setState", Params: map[string]interface{}{"state": false}})
}

func (b *Bulb) setBrightness(level int) {
	if level < 10 {
		level = 10
	} else if level > 100 {
		level = 100
	}
	b.mu.Lock()
	b.State.Brightness = level
	b.State.On = true
	b.mu.Unlock()
	b.UDPHandler.addCommand(b.IP, b.Port, MsgPayload{Method: "setPilot", Params: map[string]interface{}{"dimming": level}})
}

func (b *Bulb) setColor(rgb RGB, brightness int, cw CW) {
	b.mu.Lock()
	if brightness == -1 {
		brightness = b.State.Brightness
	}
	b.State.R, b.State.G, b.State.B = rgb.R, rgb.G, rgb.B
	b.State.C, b.State.W = cw.C, cw.W
	b.State.Brightness = brightness
	b.State.Mode = "rgb"
	b.State.On = true
	b.mu.Unlock()

	params := map[string]interface{}{
		"r": rgb.R, "g": rgb.G, "b": rgb.B,
		"c": cw.C, "w": cw.W, "dimming": brightness,
	}
	b.UDPHandler.addCommand(b.IP, b.Port, MsgPayload{Method: "setPilot", Params: params})
}

func (b *Bulb) setWhite(temp int, brightness int) {
	b.mu.Lock()
	if brightness == -1 {
		brightness = b.State.Brightness
	}
	b.State.Temp = temp
	b.State.Brightness = brightness
	b.State.Mode = "white"
	b.State.On = true
	b.mu.Unlock()

	b.UDPHandler.addCommand(b.IP, b.Port, MsgPayload{
		Method: "setPilot", Params: map[string]interface{}{"temp": temp, "dimming": brightness},
	})
}

func (b *Bulb) fetchPilot(timeout time.Duration) map[string]interface{} {
	conn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return nil
	}
	defer conn.Close()

	msg, _ := json.Marshal(MsgPayload{Method: "getPilot"})
	addr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", b.IP, b.Port))
	conn.WriteTo(msg, addr)
	conn.SetReadDeadline(time.Now().Add(timeout))

	buf := make([]byte, 1024)
	n, _, err := conn.ReadFrom(buf)
	if err != nil {
		return nil
	}

	var res map[string]interface{}
	json.Unmarshal(buf[:n], &res)
	if result, ok := res["result"].(map[string]interface{}); ok {
		return result
	}
	return nil
}

// ===== BULB MANAGEMENT & DISCOVERY ==========================================================================
type BulbManager struct {
	bulbs       []*Bulb
	udpHandler  *UDPHandler
	sceneRunner *SceneRunner
}

func (m *BulbManager) getBulbByID(id int) (*Bulb, error) {
	for _, b := range m.bulbs {
		if b.ID == id {
			return b, nil
		}
	}
	return nil, fmt.Errorf("ID %d not found", id)
}

func discoverBulbs(timeoutSec time.Duration) []string {
	msg, _ := json.Marshal(map[string]interface{}{
		"method": "registration",
		"params": map[string]interface{}{
			"phoneMac": "AAAAAAAAAAAA",
			"register": true,
			"phoneIp":  "000.000.000.000",
			"id":       "1",
		},
	})

	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil
	}
	defer conn.Close()

	if rawConn, err := conn.SyscallConn(); err == nil {
		rawConn.Control(func(fd uintptr) {
			_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
		})
	}

	addr, _ := net.ResolveUDPAddr("udp4", "255.255.255.255:38899")
	if _, err := conn.WriteTo(msg, addr); err != nil {
		return nil
	}

	conn.SetReadDeadline(time.Now().Add(timeoutSec))

	foundSet := make(map[string]bool)

	buf := make([]byte, 1024)
	for {
		_, remoteAddr, err := conn.ReadFrom(buf)
		if err != nil {
			break
		}
		ip := remoteAddr.(*net.UDPAddr).IP.String()
		foundSet[ip] = true
	}

	if len(foundSet) == 0 {
		fmt.Printf("%s%sno bulbs discovered%s\n", ErrorPrefix, FgRed, Reset)
	}

	var ips []string
	for ip := range foundSet {
		ips = append(ips, ip)
	}
	sort.Slice(ips, func(i, j int) bool {
		return ipToInt(ips[i]) < ipToInt(ips[j])
	})
	return ips
}

func ipToInt(ipStr string) uint32 {
	ip := net.ParseIP(ipStr).To4()
	if ip == nil {
		return 0
	}
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

func loadIPsFromFile(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var ips []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.FieldsFunc(line, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t'
		})
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				ips = append(ips, p)
			}
		}
	}
	return ips, scanner.Err()
}

// ===== CLI EXECUTION ============================================================================
func executeCommand(m *BulbManager, input []string) bool {
	if len(input) == 0 {
		return false
	}
	cmd := input[0]

	switch cmd {
	case "list", "ls", "l":
		fmt.Printf("\ndiscovered bulbs:\n")
		if len(m.bulbs) == 0 {
			fmt.Printf("%s%sno bulbs discovered.%s\n", ErrorPrefix, FgRed, Reset)
		}
		for _, b := range m.bulbs {
			state := b.State
			status := FgRed + "OFF" + Reset
			if state.On {
				status = FgBrtGreen + "ON" + Reset
			}
			fmt.Printf("  [%s%d%s] ip = %s%s%s status = %s\n", Bold, b.ID, Reset, FgCyan, b.IP, Reset, status)
		}

	case "discover", "ds", "d":
		fmt.Printf("\n%sdiscovering bulbs on local network...%s\n", FgCyan, Reset)
		ips := discoverBulbs(5 * time.Second)
		m.bulbs = []*Bulb{}
		for i, ip := range ips {
			m.bulbs = append(m.bulbs, NewBulb(i+1, ip, m.udpHandler, m.sceneRunner))
		}

	case "add", "a":
		if len(input) == 2 {
			rawIPs := input[1]
			ipList := strings.Split(rawIPs, ",")
			for _, ip := range ipList {
				ip = strings.TrimSpace(ip)
				if ip == "" {
					continue
				}
				newID := len(m.bulbs) + 1
				m.bulbs = append(m.bulbs, NewBulb(newID, ip, m.udpHandler, m.sceneRunner))
				fmt.Printf("bulb [%s%d%s] manually added with ip %s%s%s\n", Bold, newID, Reset, FgCyan, ip, Reset)
			}
		} else {
			fmt.Printf("%s%susage: add <ip1,ip2,...>%s\n", ErrorPrefix, FgRed, Reset)
		}

	case "on":
		if len(input) < 2 {
			fmt.Printf("%s%susage: on <id|all>%s\n", ErrorPrefix, FgRed, Reset)
			break
		}
		var targetIDs []int
		if id, err := strconv.Atoi(input[1]); err == nil {
			targetIDs = []int{id}
		} else if input[1] == "all" {
			for _, b := range m.bulbs {
				targetIDs = append(targetIDs, b.ID)
			}
		} else {
			fmt.Printf("%s%sinvalid bulb ID or target '%s'%s\n", ErrorPrefix, FgRed, input[1], Reset)
			break
		}

		for _, id := range targetIDs {
			if b, err := m.getBulbByID(id); err == nil {
				b.setOn()
				fmt.Printf("bulb [%s%d%s] turned %sON%s.\n", Bold, b.ID, Reset, FgBrtGreen, Reset)
			} else {
				fmt.Printf("%s%sbulb ID %d not found%s\n", ErrorPrefix, FgRed, id, Reset)
			}
		}

	case "off":
		if len(input) < 2 {
			fmt.Printf("%s%susage: off <id|all>%s\n", ErrorPrefix, FgRed, Reset)
			break
		}
		var targetIDs []int
		if id, err := strconv.Atoi(input[1]); err == nil {
			targetIDs = []int{id}
		} else if input[1] == "all" {
			for _, b := range m.bulbs {
				targetIDs = append(targetIDs, b.ID)
			}
		} else {
			fmt.Printf("%s%sinvalid bulb ID or target '%s'%s\n", ErrorPrefix, FgRed, input[1], Reset)
			break
		}

		for _, id := range targetIDs {
			if b, err := m.getBulbByID(id); err == nil {
				b.setOff()
				fmt.Printf("bulb [%s%d%s] turned %sOFF%s.\n", Bold, b.ID, Reset, FgRed, Reset)
			} else {
				fmt.Printf("%s%sbulb ID %d not found%s\n", ErrorPrefix, FgRed, id, Reset)
			}
		}

	case "brightness", "b":
		if len(input) < 3 {
			fmt.Printf("%s%susage: brightness <id> <10-100>%s\n", ErrorPrefix, FgRed, Reset)
			break
		}
		id, errID := strconv.Atoi(input[1])
		if errID != nil {
			fmt.Printf("%s%sinvalid or missing bulb ID: '%s'%s\n", ErrorPrefix, FgRed, input[1], Reset)
			break
		}
		level, errLvl := strconv.Atoi(input[2])
		if errLvl != nil {
			fmt.Printf("%s%sinvalid brightness level '%s'%s\n", ErrorPrefix, FgRed, input[2], Reset)
			break
		}

		if b, errB := m.getBulbByID(id); errB == nil {
			b.setBrightness(level)
			fmt.Printf("bulb [%s%d%s] brightness set to %s%d%%%s\n", Bold, b.ID, Reset, Bold, level, Reset)
		} else {
			fmt.Printf("%s%sbulb ID %d not found%s\n", ErrorPrefix, FgRed, id, Reset)
		}

	case "color", "c":
		if len(input) < 2 {
			fmt.Printf("%s%susage: color <id> [color_name]%s\n", ErrorPrefix, FgRed, Reset)
			break
		}
		id, errID := strconv.Atoi(input[1])
		if errID != nil {
			fmt.Printf("%s%sinvalid or missing bulb ID: '%s'%s\n", ErrorPrefix, FgRed, input[1], Reset)
			break
		}

		colorName := ""
		if len(input) >= 3 {
			colorName = input[2]
		} else {
			var err error
			colorName, err = fzfColorPicker()
			if err != nil || colorName == "" {
				return false
			}
		}

		if b, err := m.getBulbByID(id); err == nil {
			if rgb, ok := colorList[colorName]; ok {
				b.cancelScene()
				b.setColor(rgb, -1, CW{0, 0})
				fmt.Printf("bulb [%s%d%s] color set to %s%s%s\n", Bold, b.ID, Reset, Bold, colorName, Reset)
			} else {
				fmt.Printf("%s%sunknown color '%s'%s\n", ErrorPrefix, FgRed, colorName, Reset)
			}
		} else {
			fmt.Printf("%s%sbulb ID %d not found%s\n", ErrorPrefix, FgRed, id, Reset)
		}

	case "fzf_color", "fzf_c", "fzfc":
		if len(input) < 2 {
			fmt.Printf("%s%susage: fzf_color <id>%s\n", ErrorPrefix, FgRed, Reset)
			break
		}
		id, errID := strconv.Atoi(input[1])
		if errID != nil {
			fmt.Printf("%s%sinvalid or missing bulb ID: '%s'%s\n", ErrorPrefix, FgRed, input[1], Reset)
			break
		}
		b, err := m.getBulbByID(id)
		if err != nil {
			fmt.Printf("%s%sbulb ID %d not found%s\n", ErrorPrefix, FgRed, id, Reset)
			break
		}
		for {
			colorName, err := fzfColorPicker()
			if err != nil || colorName == "" {
				break
			}
			if rgb, ok := colorList[colorName]; ok {
				b.cancelScene()
				b.setColor(rgb, -1, CW{0, 0})
				fmt.Printf("bulb [%s%d%s] color set to %s%s%s\n", Bold, b.ID, Reset, Bold, colorName, Reset)
			} else {
				fmt.Printf("%s%sunknown color%s\n", ErrorPrefix, FgRed, Reset)
			}
		}

	case "color_picker", "color_pick", "color_p", "c_picker", "c_pick", "cp", "picker", "pick":
		if len(input) < 2 {
			fmt.Printf("%s%susage: color_picker <id>%s\n", ErrorPrefix, FgRed, Reset)
			break
		}
		id, errID := strconv.Atoi(input[1])
		if errID != nil {
			fmt.Printf("%s%sinvalid or missing bulb ID: '%s'%s\n", ErrorPrefix, FgRed, input[1], Reset)
			break
		}
		b, err := m.getBulbByID(id)
		if err != nil {
			fmt.Printf("%s%sbulb ID %d not found%s\n", ErrorPrefix, FgRed, id, Reset)
			break
		}

		rgb, err := macOSColorPicker()
		if err != nil {
			break
		}

		b.cancelScene()
		b.setColor(rgb, -1, CW{0, 0})
		fmt.Printf("bulb [%s%d%s] color set to RGB (%s%d, %d, %d%s)\n", Bold, b.ID, Reset, Bold, rgb.R, rgb.G, rgb.B, Reset)

	case "rgb":
		if len(input) < 3 {
			fmt.Printf("%s%susage: rgb <id> <R,G,B>%s\n", ErrorPrefix, FgRed, Reset)
			break
		}
		id, errID := strconv.Atoi(input[1])
		if errID != nil {
			fmt.Printf("%s%sinvalid or missing bulb ID: '%s'%s\n", ErrorPrefix, FgRed, input[1], Reset)
			break
		}
		rgbStr := input[2]

		if b, err := m.getBulbByID(id); err == nil {
			if rgb, errParse := parseRGB(rgbStr); errParse == nil {
				b.cancelScene()
				b.setColor(rgb, -1, CW{0, 0})
				fmt.Printf("bulb [%s%d%s] color set to RGB (%s%d, %d, %d%s)\n", Bold, b.ID, Reset, Bold, rgb.R, rgb.G, rgb.B, Reset)
			} else {
				fmt.Printf("%s%sinvalid RGB format.%s\n", ErrorPrefix, FgRed, Reset)
			}
		} else {
			fmt.Printf("%s%sbulb ID %d not found%s\n", ErrorPrefix, FgRed, id, Reset)
		}

	case "temp", "t":
		if len(input) < 3 {
			fmt.Printf("%s%susage: temp <id> <2200-6500>%s\n", ErrorPrefix, FgRed, Reset)
			break
		}
		id, errID := strconv.Atoi(input[1])
		if errID != nil {
			fmt.Printf("%s%sinvalid or missing bulb ID: '%s'%s\n", ErrorPrefix, FgRed, input[1], Reset)
			break
		}

		temp, err := strconv.Atoi(input[2])
		if err != nil {
			fmt.Printf("%s%sinvalid temperature format%s\n", ErrorPrefix, FgRed, Reset)
			break
		}
		if temp < 2200 || temp > 6500 {
			fmt.Printf("%s%sbulb temperature has to be between 2200-6500K%s\n", ErrorPrefix, FgRed, Reset)
		} else {
			if b, errB := m.getBulbByID(id); errB == nil {
				b.cancelScene()
				b.setWhite(temp, -1)
				fmt.Printf("bulb [%s%d%s] temperature set to %s%dK%s\n", Bold, b.ID, Reset, Bold, temp, Reset)
			} else {
				fmt.Printf("%s%sbulb ID %d not found%s\n", ErrorPrefix, FgRed, id, Reset)
			}
		}

	case "scene", "scenes", "s":
		if len(input) < 2 {
			fmt.Printf("%s%susage: scene <id> [scene_name]%s\n", ErrorPrefix, FgRed, Reset)
			break
		}
		id, errID := strconv.Atoi(input[1])
		if errID != nil {
			fmt.Printf("%s%sinvalid or missing bulb ID: '%s'%s\n", ErrorPrefix, FgRed, input[1], Reset)
			break
		}

		sceneName := ""
		if len(input) >= 3 {
			sceneName = input[2]
		} else {
			var err error
			sceneName, err = fzfScenePicker()
			if err != nil || sceneName == "" {
				return false
			}
		}

		if b, err := m.getBulbByID(id); err == nil {
			if err := b.runScene(sceneName); err != nil {
				fmt.Printf("%s%s%v%s\n", ErrorPrefix, FgRed, err, Reset)
			}
		} else {
			fmt.Printf("%s%sbulb ID %d not found%s\n", ErrorPrefix, FgRed, id, Reset)
		}

	case "stop_scene", "stop_s", "stops", "x":
		if len(input) < 2 {
			fmt.Printf("%s%susage: stop_scene <id>%s\n", ErrorPrefix, FgRed, Reset)
			break
		}
		id, errID := strconv.Atoi(input[1])
		if errID != nil {
			fmt.Printf("%s%sinvalid or missing bulb ID: '%s'%s\n", ErrorPrefix, FgRed, input[1], Reset)
			break
		}
		if b, err := m.getBulbByID(id); err == nil {
			b.cancelScene()
			fmt.Printf("bulb [%s%d%s] scene stopped\n", Bold, b.ID, Reset)
		} else {
			fmt.Printf("%s%sbulb ID %d not found%s\n", ErrorPrefix, FgRed, id, Reset)
		}

	case "fade_rgb", "f_rgb", "frgb":
		args := input[1:]
		if len(args) == 0 {
			fmt.Printf("%s%susage: fade_rgb <id> <R1,G1,B1> <R2,G2,B2> [cycle_length_s]%s\n", ErrorPrefix, FgRed, Reset)
			break
		}

		id, errID := strconv.Atoi(args[0])
		if errID != nil {
			fmt.Printf("%s%sinvalid or missing bulb ID: '%s'%s\n", ErrorPrefix, FgRed, args[0], Reset)
			break
		}

		args = args[1:]
		if len(args) == 0 {
			if _, err := m.getBulbByID(id); err != nil {
				fmt.Printf("%s%sbulb ID %d not found%s\n", ErrorPrefix, FgRed, id, Reset)
				break
			}
			rgb1, err1 := macOSColorPicker()
			if err1 != nil {
				break
			}
			rgb2, err2 := macOSColorPicker()
			if err2 != nil {
				break
			}
			c1 := fmt.Sprintf("%d,%d,%d", rgb1.R, rgb1.G, rgb1.B)
			c2 := fmt.Sprintf("%d,%d,%d", rgb2.R, rgb2.G, rgb2.B)

			fmt.Print("cycle duration in seconds (optional, press Enter to skip): ")
			scanner := bufio.NewScanner(os.Stdin)
			var duration string
			if scanner.Scan() {
				duration = strings.TrimSpace(scanner.Text())
			}
			if duration != "" {
				args = []string{c1, c2, duration}
			} else {
				args = []string{c1, c2}
			}
		}

		if len(args) >= 2 && len(args) <= 3 {
			if b, err := m.getBulbByID(id); err == nil {
				if err := b.runScene("fade_rgb", args...); err != nil {
					fmt.Printf("%s%s%v%s\n", ErrorPrefix, FgRed, err, Reset)
				}
			} else {
				fmt.Printf("%s%sbulb ID %d not found%s\n", ErrorPrefix, FgRed, id, Reset)
			}
		} else {
			fmt.Printf("%s%susage: fade_rgb <id> <R1,G1,B1> <R2,G2,B2> [cycle_length_s]%s\n", ErrorPrefix, FgRed, Reset)
		}

	case "fade_color", "fade_c", "f_c", "fc":
		args := input[1:]
		if len(args) == 0 {
			fmt.Printf("%s%susage: fade_color <id> <color1> <color2> [cycle_length_s]%s\n", ErrorPrefix, FgRed, Reset)
			break
		}

		id, errID := strconv.Atoi(args[0])
		if errID != nil {
			fmt.Printf("%s%sinvalid or missing bulb ID: '%s'%s\n", ErrorPrefix, FgRed, args[0], Reset)
			break
		}

		args = args[1:]
		if len(args) == 0 {
			if _, err := m.getBulbByID(id); err != nil {
				fmt.Printf("%s%sbulb ID %d not found%s\n", ErrorPrefix, FgRed, id, Reset)
				break
			}
			c1, err1 := fzfColorPicker()
			if err1 != nil || c1 == "" {
				break
			}
			c2, err2 := fzfColorPicker()
			if err2 != nil || c2 == "" {
				break
			}

			fmt.Print("cycle duration in seconds (optional, press Enter to skip): ")
			scanner := bufio.NewScanner(os.Stdin)
			var duration string
			if scanner.Scan() {
				duration = strings.TrimSpace(scanner.Text())
			}
			if duration != "" {
				args = []string{c1, c2, duration}
			} else {
				args = []string{c1, c2}
			}
		}

		if len(args) >= 2 && len(args) <= 3 {
			if b, err := m.getBulbByID(id); err == nil {
				if err := b.runScene("fade_color", args...); err != nil {
					fmt.Printf("%s%s%v%s\n", ErrorPrefix, FgRed, err, Reset)
				}
			} else {
				fmt.Printf("%s%sbulb ID %d not found%s\n", ErrorPrefix, FgRed, id, Reset)
			}
		} else {
			fmt.Printf("%s%susage: fade_color <id> <color1> <color2> [cycle_length_s]%s\n", ErrorPrefix, FgRed, Reset)
		}

	case "reset", "r":
		if len(input) < 2 {
			fmt.Printf("%s%susage: reset <id>%s\n", ErrorPrefix, FgRed, Reset)
			break
		}
		id, errID := strconv.Atoi(input[1])
		if errID != nil {
			fmt.Printf("%s%sinvalid or missing bulb ID: '%s'%s\n", ErrorPrefix, FgRed, input[1], Reset)
			break
		}
		if b, err := m.getBulbByID(id); err == nil {
			if err := b.runScene("reset"); err != nil {
				fmt.Printf("%s%s%v%s\n", ErrorPrefix, FgRed, err, Reset)
			}
		} else {
			fmt.Printf("%s%sbulb ID %d not found%s\n", ErrorPrefix, FgRed, id, Reset)
		}

	case "poll", "p":
		if len(input) < 2 {
			fmt.Printf("%s%susage: poll <id>%s\n", ErrorPrefix, FgRed, Reset)
			break
		}
		id, errID := strconv.Atoi(input[1])
		if errID != nil {
			fmt.Printf("%s%sinvalid or missing bulb ID: '%s'%s\n", ErrorPrefix, FgRed, input[1], Reset)
			break
		}
		if b, err := m.getBulbByID(id); err == nil {
			fmt.Printf("polling bulb [%s%d%s] at %s...\n", Bold, b.ID, Reset, b.IP)
			if pilot := b.fetchPilot(2 * time.Second); pilot != nil {
				fmt.Printf("%sresponse:%s\n", FgCyan, Reset)
				j, _ := json.MarshalIndent(pilot, "", "  ")
				fmt.Println(string(j))
			} else {
				fmt.Printf("%s%sno response received (timeout)%s\n", ErrorPrefix, FgRed, Reset)
			}
		} else {
			fmt.Printf("%s%sbulb ID %d not found%s\n", ErrorPrefix, FgRed, id, Reset)
		}

	case "color_list", "color_l", "c_l", "cl":
		fmt.Printf("\navailable colors:\n")
		for name := range colorList {
			fmt.Printf("  - %s\n", name)
		}

	case "scene_list", "scene_l", "s_l", "sl":
		fmt.Printf("\navailable scenes:\n")
		for sName := range loadedScenes {
			fmt.Printf("  - %s%s%s: %s\n", Bold, sName, Reset, loadedScenes[sName].Description)
		}
		fmt.Printf("  - default scene %sreset%s: resets the bulb to a static default state (dim cold white)\n", Bold, Reset)
		fmt.Printf("  - default scene %sfade_rgb%s: fade between two custom RGB values\n", Bold, Reset)
		fmt.Printf("  - default scene %sfade_color%s: fade between two named colors\n", Bold, Reset)

	case "load_scene_file", "load_s_f", "load_sf", "l_s_f", "l_sf", "lsf":
		targetFile := "scenes.toml"
		if len(input) >= 2 {
			targetFile = input[1]
		}

		filePath := targetFile
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			exePath, errExe := os.Executable()
			if errExe == nil {
				binDir := filepath.Dir(exePath)
				appBundleResPath := filepath.Join(binDir, "..", "Resources", targetFile)
				localPath := filepath.Join(binDir, targetFile)

				if _, err := os.Stat(appBundleResPath); err == nil {
					filePath = appBundleResPath
				} else if _, err := os.Stat(localPath); err == nil {
					filePath = localPath
				}
			}
		}

		if err := loadScenesFromFile(filePath); err != nil {
			fmt.Printf("%s%scould not load scenes from %s: %v%s\n", ErrorPrefix, FgRed, filePath, err, Reset)
		} else {
			fmt.Printf("scenes loaded from file: %s\n", filePath)
		}

	case "load_color_file", "load_c_f", "load_cf", "l_c_f", "l_cf", "lcf":
		if len(input) == 2 {
			if input[1] == "add" || input[1] == "default_colors_expanded" || input[1] == "dce" {
				exePath, err := os.Executable()
				if err != nil {
					fmt.Printf("%s%scould not find executable path: %v%s\n", ErrorPrefix, FgRed, err, Reset)
					break
				}
				binDir := filepath.Dir(exePath)
				appBundleResPath := filepath.Join(binDir, "..", "Resources", "default_colors_expanded.toml")
				localPath := filepath.Join(binDir, "default_colors_expanded.toml")
				var colorsFilePath string
				if _, err := os.Stat(appBundleResPath); err == nil {
					colorsFilePath = appBundleResPath
				} else {
					colorsFilePath = localPath
				}
				loadColorsFromFile(colorsFilePath)
				fmt.Printf("colors loaded from file: %s\n", colorsFilePath)
			} else {
				loadColorsFromFile(input[1])
				fmt.Printf("colors loaded from file: %s\n", input[1])
			}
		}

	case "quit", "exit", "q":
		return true

	default:
		fmt.Printf("%s%sinvalid command or arguments.%s\n", ErrorPrefix, FgRed, Reset)
	}
	return false
}

func printVersion() {
	fmt.Printf("version: %s%s%s\n", Bold, version, Reset)
}

func printHelp() {
	fmt.Printf("\n%s%s%s *** WiZ-CLI Control - daemon version *** %s\n", Bold, FgWhite, BgGrey, Reset)
	fmt.Printf("%s------------------------------------------------------------------------------------%s\n", FgYellow, Reset)
	fmt.Println("available commands:")
	fmt.Printf("  %slist%s                         - list discovered bulbs\n", FgYellow, Reset)
	fmt.Printf("  %sdiscover%s                     - re-discover bulbs on local network\n", FgYellow, Reset)
	fmt.Printf("  %sadd%s <ip1,ip2,...>            - manually add bulb(s) by IP address(es)\n", FgYellow, Reset)
	fmt.Printf("  %son%s <id|all>                  - turn specified bulb or all bulbs ON\n", FgYellow, Reset)
	fmt.Printf("  %soff%s <id|all>                 - turn specified bulb or all bulbs OFF\n", FgYellow, Reset)
	fmt.Printf("  %sbrightness%s <id> <10-100>     - set bulb brightness percentage\n", FgYellow, Reset)
	fmt.Printf("  %scolor%s <id> [color_name]      - set bulb to a color (e.g. red, green)\n", FgYellow, Reset)
	fmt.Printf("  %s%s                             - fzf color picker runs when no [color_name] specified\n", FgYellow, Reset)
	fmt.Printf("  %sfzf_color%s <id>               - interactively browse and apply colors until ESC\n", FgYellow, Reset)
	fmt.Printf("  %scolor_picker%s <id>            - open macOS color picker to set bulb color\n", FgYellow, Reset)
	fmt.Printf("  %srgb%s <id> <R,G,B>             - set bulb RGB value (e.g. rgb 1 255,128,0)\n", FgYellow, Reset)
	fmt.Printf("  %stemp%s <id> <2200-6500>        - set bulb to white, temperature in Kelvin\n", FgYellow, Reset)
	fmt.Printf("  %sscene%s <id> [scene_name]      - run scene on bulb\n", FgYellow, Reset)
	fmt.Printf("  %s%s                             - fzf scene picker runs when no [scene_name] specified\n", FgYellow, Reset)
	fmt.Printf("  %sstop_scene%s <id>              - stop active scene or fade on bulb\n", FgYellow, Reset)
	fmt.Printf("  %sfade_rgb%s <id> [R1,G1,B1] [R2,G2,B2] [cycle_length_s]\n", FgYellow, Reset)
	fmt.Printf("  %s%s                             - fade between two RGB values\n", FgYellow, Reset)
	fmt.Printf("  %sfade_color%s <id> [color1] [color2] [cycle_length_s]\n", FgYellow, Reset)
	fmt.Printf("  %s%s                             - fade between two named colors\n", FgYellow, Reset)
	fmt.Printf("  %sreset%s <id>                   - run reset scene on bulb\n", FgYellow, Reset)
	fmt.Printf("  %spoll%s <id>                    - poll bulb for physical device state\n", FgYellow, Reset)
	fmt.Printf("  %scolor_list%s                   - list available bulb colors\n", FgYellow, Reset)
	fmt.Printf("  %sscene_list%s                   - list available bulb scenes\n", FgYellow, Reset)
	fmt.Printf("  %sload_color_file%s <path>       - load colors from file (.toml / .colors)\n", FgYellow, Reset)
	fmt.Printf("  %sload_scene_file%s [path]       - load scenes from file (.toml / .scenes)\n", FgYellow, Reset)
	fmt.Printf("  %squit%s                         - exit the program\n", FgYellow, Reset)
	fmt.Printf("%s! note: commands can be abbreviated%s\n", FgGrey, Reset)
	fmt.Println()
	fmt.Println("startup flags (terminal CLI):")
	fmt.Printf("  wiz-cli %s-file%s <path>         - load bulb IPs from file\n", FgYellow, Reset)
	fmt.Printf("  wiz-cli %s-ips%s <ip1,ip2,...>   - specify comma-separated bulb IPs\n", FgYellow, Reset)
	fmt.Println()
	fmt.Println("daemon control (terminal CLI):")
	fmt.Printf("  wiz-cli %sdaemon status%s        - show status of background process\n", FgYellow, Reset)
	fmt.Printf("  wiz-cli %sdaemon stop%s          - cancel all running scenes and terminate the daemon\n", FgYellow, Reset)
	fmt.Printf("%s------------------------------------------------------------------------------------%s\n", FgYellow, Reset)
}

// ===== DAEMON ====================================================================================
type DaemonRequest struct {
	IPs     []string `json:"ips,omitempty"`
	Command []string `json:"command"`
}

type DaemonResponse struct {
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

func daemonSocketPath() string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("wiz-cli-%d.sock", os.Getuid()))
}

func daemonIsRunning() bool {
	conn, err := net.DialTimeout("unix", daemonSocketPath(), 150*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func startDaemon() error {
	if daemonIsRunning() {
		return nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer devNull.Close()

	cmd := exec.Command(exePath, "--daemon")
	cmd.Stdin = devNull
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if daemonIsRunning() {
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not start")
}

func sendDaemonRequest(req DaemonRequest) (DaemonResponse, error) {
	conn, err := net.DialTimeout("unix", daemonSocketPath(), 500*time.Millisecond)
	if err != nil {
		return DaemonResponse{}, err
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return DaemonResponse{}, err
	}

	var res DaemonResponse
	if err := json.NewDecoder(conn).Decode(&res); err != nil {
		return DaemonResponse{}, err
	}
	return res, nil
}

func sameBulbIPs(m *BulbManager, ips []string) bool {
	if len(m.bulbs) != len(ips) {
		return false
	}
	for i, ip := range ips {
		if m.bulbs[i].IP != ip {
			return false
		}
	}
	return true
}

func configureDaemonBulbs(m *BulbManager, ips []string) {
	if len(ips) > 0 {
		if sameBulbIPs(m, ips) {
			return
		}
		m.sceneRunner.cancelAll()
		m.bulbs = nil
		for i, ip := range ips {
			m.bulbs = append(m.bulbs, NewBulb(i+1, ip, m.udpHandler, m.sceneRunner))
		}
		return
	}

	// No IPs were supplied. Only discover when the daemon has no bulb set yet.
	if len(m.bulbs) == 0 {
		fmt.Printf("%s%sdiscovering bulbs on local network...%s\n", Bold, FgCyan, Reset)
		discovered := discoverBulbs(4 * time.Second)
		for i, ip := range discovered {
			m.bulbs = append(m.bulbs, NewBulb(i+1, ip, m.udpHandler, m.sceneRunner))
		}
	}
}

func captureStdout(fn func()) string {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		fn()
		return ""
	}

	os.Stdout = w
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	fn()

	_ = w.Close()
	os.Stdout = oldStdout
	<-done
	_ = r.Close()
	return buf.String()
}

func runDaemon() error {
	socketPath := daemonSocketPath()
	_ = os.Remove(socketPath) // remove a stale socket after an unclean shutdown

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	defer listener.Close()
	defer os.Remove(socketPath)
	_ = os.Chmod(socketPath, 0600)

	udpHandler := newUDPHandler(100)
	sceneRunner := newSceneRunner()
	manager := &BulbManager{udpHandler: udpHandler, sceneRunner: sceneRunner}

	// Cleanly remove the Unix socket on Ctrl-C / termination.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		sceneRunner.cancelAll()
		_ = listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			return nil
		}

		var req DaemonRequest
		if err := json.NewDecoder(conn).Decode(&req); err != nil {
			_ = json.NewEncoder(conn).Encode(DaemonResponse{Error: err.Error()})
			conn.Close()
			continue
		}

		if len(req.Command) == 1 && req.Command[0] == "__daemon_stop__" {
			sceneRunner.cancelAll()
			_ = json.NewEncoder(conn).Encode(DaemonResponse{Output: "wiz-cli daemon stopped\n"})
			conn.Close()
			return nil
		}

		output := captureStdout(func() {
			configureDaemonBulbs(manager, req.IPs)
			if len(req.Command) > 0 {
				executeCommand(manager, req.Command)
			}
		})

		_ = json.NewEncoder(conn).Encode(DaemonResponse{Output: output})
		conn.Close()
	}
}

func loadDefaultSceneFile() {
	if exePath, err := os.Executable(); err == nil {
		binDir := filepath.Dir(exePath)
		appBundleResPath := filepath.Join(binDir, "..", "Resources", "scenes.toml")
		localPath := filepath.Join(binDir, "scenes.toml")
		if _, err := os.Stat(appBundleResPath); err == nil {
			_ = loadScenesFromFile(appBundleResPath)
		} else if _, err := os.Stat(localPath); err == nil {
			_ = loadScenesFromFile(localPath)
		}
	}
}

func parseStartupArgs(args []string) (bulbFile string, ipFlag string, cliArgs []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if (arg == "-file" || arg == "-f" || arg == "--file") && i+1 < len(args) {
			bulbFile = args[i+1]
			i++
		} else if (arg == "-ips" || arg == "-i" || arg == "--ips") && i+1 < len(args) {
			ipFlag = args[i+1]
			i++
		} else {
			cliArgs = append(cliArgs, strings.ToLower(arg))
		}
	}
	return
}

func collectStartupIPs(bulbFile, ipFlag string) []string {
	var ips []string

	if bulbFile != "" {
		if fromFile, err := loadIPsFromFile(bulbFile); err == nil {
			ips = append(ips, fromFile...)
		} else {
			fmt.Printf("%s%scould not load bulb file %s: %v%s\n", ErrorPrefix, FgRed, bulbFile, err, Reset)
		}
	}

	if ipFlag != "" {
		for _, ip := range strings.Split(ipFlag, ",") {
			ip = strings.TrimSpace(ip)
			if ip != "" {
				ips = append(ips, ip)
			}
		}
	}
	return ips
}

func main() {
	args := os.Args[1:]

	// Internal daemon process. Users normally never invoke this directly.
	if len(args) == 1 && args[0] == "--daemon" {
		loadDefaultSceneFile()
		if err := runDaemon(); err != nil {
			fmt.Fprintf(os.Stderr, "%sdaemon error: %v\n", ErrorPrefix, err)
			os.Exit(1)
		}
		return
	}

	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		printHelp()
		fmt.Println("daemon commands:")
		fmt.Println("  daemon status                  - show whether the background daemon is running")
		fmt.Println("  daemon stop                    - stop the background daemon and all active scenes")
		return
	}
	if len(args) > 0 && (args[0] == "-v" || args[0] == "--version") {
		printVersion()
		return
	}

	if len(args) >= 2 && args[0] == "daemon" && args[1] == "status" {
		if daemonIsRunning() {
			fmt.Println("wiz-cli daemon is running")
		} else {
			fmt.Println("wiz-cli daemon is not running")
		}
		return
	}

	if len(args) >= 2 && args[0] == "daemon" && args[1] == "stop" {
		if !daemonIsRunning() {
			fmt.Println("wiz-cli daemon is not running")
			return
		}
		res, err := sendDaemonRequest(DaemonRequest{Command: []string{"__daemon_stop__"}})
		if err != nil {
			fmt.Printf("%s%scould not stop daemon: %v%s\n", ErrorPrefix, FgRed, err, Reset)
			return
		}
		fmt.Print(res.Output)
		return
	}

	bulbFile, ipFlag, cliArgs := parseStartupArgs(args)
	initialIPs := collectStartupIPs(bulbFile, ipFlag)

	// One-shot command mode: send the command to the persistent daemon and exit.
	if len(cliArgs) > 0 {
		if err := startDaemon(); err != nil {
			fmt.Printf("%s%scould not start daemon: %v%s\n", ErrorPrefix, FgRed, err, Reset)
			os.Exit(1)
		}

		res, err := sendDaemonRequest(DaemonRequest{IPs: initialIPs, Command: cliArgs})
		if err != nil {
			fmt.Printf("%s%sdaemon request failed: %v%s\n", ErrorPrefix, FgRed, err, Reset)
			os.Exit(1)
		}
		if res.Error != "" {
			fmt.Printf("%s%s%s%s\n", ErrorPrefix, FgRed, res.Error, Reset)
			os.Exit(1)
		}
		fmt.Print(res.Output)
		return
	}

	// No command arguments: preserve the original interactive mode as a standalone process.
	loadDefaultSceneFile()
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	fmt.Printf("%s%sWiZ-CLI - PROGRAM START%s\nversion: %s\n\n", Bold, Underline, Reset, version)

	conn, err := net.ListenPacket("udp4", ":38899")
	if err != nil {
		fmt.Printf("%s%scould not initialise UDP / network permissions%s\n", ErrorPrefix, FgRed, Reset)
	} else {
		defer conn.Close()
	}

	udpHandler := newUDPHandler(100)
	sceneRunner := newSceneRunner()
	setupSignalHandler(cancel, sceneRunner)
	manager := &BulbManager{udpHandler: udpHandler, sceneRunner: sceneRunner}

	if len(initialIPs) > 0 {
		for i, ip := range initialIPs {
			manager.bulbs = append(manager.bulbs, NewBulb(i+1, ip, udpHandler, sceneRunner))
		}
	} else {
		fmt.Printf("%s%sdiscovering bulbs on local network...\n%s", Bold, FgCyan, Reset)
		discovered := discoverBulbs(4 * time.Second)
		for i, ip := range discovered {
			manager.bulbs = append(manager.bulbs, NewBulb(i+1, ip, udpHandler, sceneRunner))
		}
	}

	printHelp()
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Printf("\n%s%s%s wiz-cli: %s ", Bold, FgWhite, BgGrey, Reset)
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		fields := strings.Fields(strings.ToLower(line))
		if len(fields) > 0 && executeCommand(manager, fields) {
			break
		}
	}
	fmt.Printf("\n%s%sPROGRAM END%s\n\n", Bold, Underline, Reset)
}
