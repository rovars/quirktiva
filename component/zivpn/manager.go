package zivpn

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/yaling888/quirktiva/log"
)

var (
	instanceMu      sync.Mutex
	instanceCleanup func()
)

const defaultObfs = "hu``hqb`c"

func pipeLog(rc io.ReadCloser) {
	scanner := bufio.NewScanner(rc)
	for scanner.Scan() {
		trimmed := strings.TrimSpace(scanner.Text())
		if trimmed != "" {
			log.Infoln("%s", trimmed)
		}
	}
}

// StartInstance starts a ZIVPN cluster and returns the listening port, a cleanup function, and error
// It ensures only one instance is running globally.
func StartInstance(homeDir, name, host, portRange, password, obfs string, workers int, recvWindowConn, recvWindow int, udpEnabled bool, up string, upMbps int, down string, downMbps int) (int, func(), error) {
	instanceMu.Lock()
	defer instanceMu.Unlock()

	// Hard cleanup: Kill any existing libuz/libload to prevent orphans
	_ = exec.Command("pkill", "-9", "-f", "libuz").Run()
	_ = exec.Command("pkill", "-9", "-f", "libload").Run()

	// Kill previous instance cleanup if any
	if instanceCleanup != nil {
		log.Infoln("[ZIVPN] Stopping previous managed instance...")
		instanceCleanup()
		instanceCleanup = nil
	}

	// Give OS a moment to release ports
	time.Sleep(500 * time.Millisecond)

	// Workers logic: Default to 2 if not set, allow 1, max 10
	if workers <= 0 {
		workers = 2
	} else if workers > 10 {
		workers = 10
	}

	if obfs == "" {
		obfs = defaultObfs
	}

	if recvWindowConn <= 0 {
		recvWindowConn = 131072
	}

	if recvWindow <= 0 {
		recvWindow = 327680
	}

	libuzPath := filepath.Join(homeDir, "libuz")
	libloadPath := filepath.Join(homeDir, "libload")

	if _, err := os.Stat(libuzPath); os.IsNotExist(err) {
		return 0, nil, fmt.Errorf("libuz binary not found at %s", libuzPath)
	}
	if _, err := os.Stat(libloadPath); os.IsNotExist(err) {
		return 0, nil, fmt.Errorf("libload binary not found at %s", libloadPath)
	}

	// Ensure binaries are executable
	_ = os.Chmod(libuzPath, 0755)
	_ = os.Chmod(libloadPath, 0755)

	// Get main port for load balancer
	ports, err := getFreePorts(1)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to get free port: %w", err)
	}
	mainPort := ports[0]

	log.Infoln("[ZIVPN] %s: Initializing cluster with %d workers on port %d...", name, workers, mainPort)

	var cmds []*exec.Cmd
	var muCmds sync.Mutex

	cleanup := func() {
		muCmds.Lock()
		defer muCmds.Unlock()
		for _, cmd := range cmds {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}
		}
		cmds = nil
	}

	// Register current cleanup
	instanceCleanup = cleanup

	// Get free ports for workers
	workerPorts, err := getFreePorts(workers)
	if err != nil {
		cleanup()
		return 0, nil, fmt.Errorf("failed to get free ports for workers: %w", err)
	}

	tunnelArgs := []string{"-lport", fmt.Sprintf("%d", mainPort), "-tunnel"}

	// Prepare environment
	env := os.Environ()
	env = append(env, fmt.Sprintf("LD_LIBRARY_PATH=%s", homeDir))

	// Smart Server Address Construction
	serverAddr := host
	if portRange != "" {
		if !strings.HasPrefix(portRange, ":") {
			serverAddr += ":" + portRange
		} else {
			serverAddr += portRange
		}
	} else {
		// Only append default range if no port is present in host
		hasPort := strings.Contains(host, ":")
		if strings.HasPrefix(host, "[") && strings.Contains(host, "]") {
			// IPv6 handling: check if there's a colon after the closing bracket
			hasPort = strings.Contains(host[strings.Index(host, "]"):], ":")
		}
		if !hasPort {
			serverAddr = host + ":6000-19999"
		}
	}

	// Build bandwidth part of the JSON
	var bwParts []string
	if up != "" {
		bwParts = append(bwParts, fmt.Sprintf(`"up": "%s"`, up))
	} else if upMbps > 0 {
		bwParts = append(bwParts, fmt.Sprintf(`"up_mbps": %d`, upMbps))
	}

	if down != "" {
		bwParts = append(bwParts, fmt.Sprintf(`"down": "%s"`, down))
	} else if downMbps > 0 {
		bwParts = append(bwParts, fmt.Sprintf(`"down_mbps": %d`, downMbps))
	}

	var bandwidthJSON string
	if len(bwParts) > 0 {
		bandwidthJSON = fmt.Sprintf(`, "bandwidth": {%s}`, strings.Join(bwParts, ", "))
	}

	for i, port := range workerPorts {
		// Construct Config JSON
		config := fmt.Sprintf(`{"server":"%s","obfs":"%s","auth":"%s","socks5":{"listen":"127.0.0.1:%d","disable_udp":%t},"insecure":true,"recvwindowconn":%d,"recvwindow":%d%s}`,
			serverAddr, obfs, password, port, !udpEnabled, recvWindowConn, recvWindow, bandwidthJSON)

		// Put back the -s flag which is likely mandatory for the binary's parser
		cmd := exec.Command(libuzPath, "-s", obfs, "--config", config)
		cmd.Env = env
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Pdeathsig: syscall.SIGKILL,
		}

		if err := cmd.Start(); err != nil {
			cleanup()
			return 0, nil, fmt.Errorf("failed to start libuz worker %d: %w", i+1, err)
		}

		muCmds.Lock()
		cmds = append(cmds, cmd)
		muCmds.Unlock()

		tunnelArgs = append(tunnelArgs, fmt.Sprintf("127.0.0.1:%d", port))
	}

	// Wait and verify workers are ready
	for _, port := range workerPorts {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		success := false
		for attempt := 0; attempt < 5; attempt++ {
			conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
			if err == nil {
				conn.Close()
				success = true
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		if !success {
			log.Warnln("[ZIVPN] Worker on port %d failed to start in time", port)
		}
	}

	// Start Load Balancer
	cmdLoad := exec.Command(libloadPath, tunnelArgs...)
	cmdLoad.Env = env
	cmdLoad.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
	}

	stdout, _ := cmdLoad.StdoutPipe()
	stderr, _ := cmdLoad.StderrPipe()
	go pipeLog(stdout)
	go pipeLog(stderr)

	if err := cmdLoad.Start(); err != nil {
		cleanup()
		return 0, nil, fmt.Errorf("failed to start libload: %w", err)
	}

	muCmds.Lock()
	cmds = append(cmds, cmdLoad)
	muCmds.Unlock()

	return mainPort, cleanup, nil
}

// Helper to get n free ports
func getFreePorts(count int) ([]int, error) {
	var ports []int
	var listeners []net.Listener

	// Open listeners to reserve ports
	for i := 0; i < count; i++ {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			// Close opened ones
			for _, l := range listeners {
				l.Close()
			}
			return nil, err
		}
		listeners = append(listeners, l)
		ports = append(ports, l.Addr().(*net.TCPAddr).Port)
	}

	// Close all listeners so ports are available for libuz
	for _, l := range listeners {
		l.Close()
	}
	return ports, nil
}
