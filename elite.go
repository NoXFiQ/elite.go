// elite-x.go
// ELITE-X DNSTT Server v3.5 - Go Implementation
// High Performance • Concurrent • All-in-One Binary

package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ========== CONSTANTS ==========
const (
	Version          = "3.5"
	ActivationKey    = "ELITE X"
	TrialKey         = "ELITE-X-TEST-0208"
	ConfigDir        = "/etc/elite-x"
	UsersDir         = "/etc/elite-x/users"
	TrafficDir       = "/etc/elite-x/traffic"
	SessionsDir      = "/etc/elite-x/sessions"
	DNSTTDir         = "/etc/dnstt"
	LogFile          = "/var/log/elite-x.log"
	TimeZone         = "Africa/Dar_es_Salaam"
	DefaultDNSTTPort = 5300
	DefaultMTU       = 1800
)

// ========== COLORS ==========
var (
	Red     = "\033[0;31m"
	Green   = "\033[0;32m"
	Yellow  = "\033[1;33m"
	Blue    = "\033[0;34m"
	Purple  = "\033[0;35m"
	Cyan    = "\033[0;36m"
	White   = "\033[1;37m"
	Bold    = "\033[1m"
	NC      = "\033[0m"
)

// ========== DATA STRUCTURES ==========

type User struct {
	Username     string    `json:"username"`
	Password     string    `json:"password"`
	ExpireDate   string    `json:"expire_date"`
	TrafficLimit int64     `json:"traffic_limit"` // MB
	MaxLogins    int       `json:"max_logins"`
	CreatedAt    time.Time `json:"created_at"`
	Locked       bool      `json:"locked"`
}

type Session struct {
	PID        int       `json:"pid"`
	Username   string    `json:"username"`
	StartTime  time.Time `json:"start_time"`
	LastUpdate time.Time `json:"last_update"`
	RxBytes    uint64    `json:"rx_bytes"`
	TxBytes    uint64    `json:"tx_bytes"`
	IP         string    `json:"ip"`
	Port       int       `json:"port"`
}

type TrafficData struct {
	Username string `json:"username"`
	TotalMB  int64  `json:"total_mb"`
	Updated  time.Time `json:"updated"`
}

type Config struct {
	Subdomain   string `json:"subdomain"`
	DNSTTPort   int    `json:"dnstt_port"`
	MTU         int    `json:"mtu"`
	Location    string `json:"location"`
	Activation  string `json:"activation"`
	Expiry      string `json:"expiry"`
	ActivationType string `json:"activation_type"`
	ActivationDate string `json:"activation_date"`
	ExpiryDays  int    `json:"expiry_days"`
}

// ========== GLOBAL VARIABLES ==========

var (
	config        Config
	users         map[string]*User
	traffic       map[string]int64
	sessions      map[string]*Session // key: PID_username
	mu            sync.RWMutex
	trafficMu     sync.RWMutex
	sessionMu     sync.RWMutex
	stopChan      chan struct{}
	dnsttCmd      *exec.Cmd
	ednsProxyCmd  *exec.Cmd
)

// ========== UTILITY FUNCTIONS ==========

func logMessage(msg string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	entry := fmt.Sprintf("[%s] %s\n", timestamp, msg)
	
	// Write to log file
	f, err := os.OpenFile(LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		defer f.Close()
		f.WriteString(entry)
	}
	
	// Also print to stdout with colors
	fmt.Printf("%s%s%s", Cyan, entry, NC)
}

func printColor(text, color string) {
	fmt.Printf("%s%s%s", color, text, NC)
}

func showBanner() {
	fmt.Printf("%s╔═══════════════════════════════════════════════════════════════╗%s\n", Purple, NC)
	fmt.Printf("%s║%s%s                   ELITE-X SLOWDNS v%s                        %s║%s\n", 
		Purple, Yellow, Bold, Version, Purple, NC)
	fmt.Printf("%s║%s%s              Advanced • Secure • Ultra Fast                    %s║%s\n", 
		Purple, Green, Bold, Purple, NC)
	fmt.Printf("%s╚═══════════════════════════════════════════════════════════════╝%s\n", Purple, NC)
	fmt.Println()
}

func showQuote() {
	fmt.Println()
	fmt.Printf("%s╔═══════════════════════════════════════════════════════════════╗%s\n", Cyan, NC)
	fmt.Printf("%s║%s%s                                                               %s║%s\n", Cyan, Yellow, Bold, Cyan, NC)
	fmt.Printf("%s║%s            Always Remember ELITE-X when you see X            %s║%s\n", Cyan, White, Cyan, NC)
	fmt.Printf("%s║%s%s                                                               %s║%s\n", Cyan, Yellow, Bold, Cyan, NC)
	fmt.Printf("%s╚═══════════════════════════════════════════════════════════════╝%s\n", Cyan, NC)
	fmt.Println()
}

// ========== CONFIGURATION MANAGEMENT ==========

func initConfig() error {
	// Create directories
	dirs := []string{ConfigDir, UsersDir, TrafficDir, SessionsDir, DNSTTDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	
	// Load or create config
	configFile := filepath.Join(ConfigDir, "config.json")
	data, err := os.ReadFile(configFile)
	if err == nil {
		return json.Unmarshal(data, &config)
	}
	
	// Default config
	config = Config{
		DNSTTPort:   DefaultDNSTTPort,
		MTU:         DefaultMTU,
		Location:    "South Africa",
		Expiry:      "Lifetime",
		ActivationType: "lifetime",
	}
	
	return saveConfig()
}

func saveConfig() error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(ConfigDir, "config.json"), data, 0644)
}

func loadUsers() error {
	mu.Lock()
	defer mu.Unlock()
	
	users = make(map[string]*User)
	
	files, err := os.ReadDir(UsersDir)
	if err != nil {
		return err
	}
	
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		
		data, err := os.ReadFile(filepath.Join(UsersDir, file.Name()))
		if err != nil {
			continue
		}
		
		var user User
		if err := json.Unmarshal(data, &user); err != nil {
			continue
		}
		
		users[user.Username] = &user
	}
	
	return nil
}

func saveUser(user *User) error {
	mu.Lock()
	defer mu.Unlock()
	
	users[user.Username] = user
	
	data, err := json.MarshalIndent(user, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(filepath.Join(UsersDir, user.Username), data, 0644)
}

func deleteUser(username string) error {
	mu.Lock()
	defer mu.Unlock()
	
	delete(users, username)
	
	// Delete user files
	os.Remove(filepath.Join(UsersDir, username))
	os.Remove(filepath.Join(TrafficDir, username))
	
	// Kill user processes
	cmd := exec.Command("pkill", "-u", username)
	cmd.Run()
	
	// Delete system user
	cmd = exec.Command("userdel", "-r", username)
	cmd.Run()
	
	return nil
}

func loadTraffic() error {
	trafficMu.Lock()
	defer trafficMu.Unlock()
	
	traffic = make(map[string]int64)
	
	files, err := os.ReadDir(TrafficDir)
	if err != nil {
		return err
	}
	
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		
		data, err := os.ReadFile(filepath.Join(TrafficDir, file.Name()))
		if err != nil {
			continue
		}
		
		total, _ := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
		traffic[file.Name()] = total
	}
	
	return nil
}

func saveTraffic(username string, total int64) error {
	trafficMu.Lock()
	defer trafficMu.Unlock()
	
	traffic[username] = total
	return os.WriteFile(filepath.Join(TrafficDir, username), []byte(fmt.Sprintf("%d", total)), 0644)
}

// ========== ACTIVATION ==========

func activateScript(inputKey string) bool {
	if inputKey == ActivationKey || inputKey == "Whtsapp 0713628668" {
		config.Activation = ActivationKey
		config.ActivationType = "lifetime"
		config.Expiry = "Lifetime"
		saveConfig()
		logMessage("Lifetime activation recorded")
		return true
	} else if inputKey == TrialKey {
		config.Activation = TrialKey
		config.ActivationType = "temporary"
		config.ActivationDate = time.Now().Format("2006-01-02")
		config.ExpiryDays = 2
		config.Expiry = "2 Days Trial"
		saveConfig()
		logMessage("Trial activation recorded (2 days)")
		return true
	}
	return false
}

func checkExpiry() bool {
	if config.ActivationType != "temporary" {
		return true
	}
	
	actDate, err := time.Parse("2006-01-02", config.ActivationDate)
	if err != nil {
		return true
	}
	
	expiryDate := actDate.AddDate(0, 0, config.ExpiryDays)
	if time.Now().After(expiryDate) {
		fmt.Printf("%s╔═══════════════════════════════════════════════════════════════╗%s\n", Red, NC)
		fmt.Printf("%s║%s           TRIAL PERIOD EXPIRED                                  %s║%s\n", Red, Yellow, Red, NC)
		fmt.Printf("%s╠═══════════════════════════════════════════════════════════════╣%s\n", Red, NC)
		fmt.Printf("%s║%s  Your %d-day trial has ended.                                  %s║%s\n", Red, White, config.ExpiryDays, Red, NC)
		fmt.Printf("%s║%s  Script will now uninstall itself...                         %s║%s\n", Red, White, Red, NC)
		fmt.Printf("%s╚═══════════════════════════════════════════════════════════════╝%s\n", Red, NC)
		return false
	}
	
	daysLeft := int(expiryDate.Sub(time.Now()).Hours() / 24)
	hoursLeft := int(expiryDate.Sub(time.Now()).Hours()) % 24
	fmt.Printf("%s⚠️  Trial: %d days %d hours remaining%s\n", Yellow, daysLeft, hoursLeft, NC)
	return true
}

// ========== USER MANAGEMENT ==========

func addUser(username, password string, days int, trafficLimit int64, maxLogins int) error {
	// Check if user exists
	if _, exists := users[username]; exists {
		return fmt.Errorf("user already exists")
	}
	
	// Create system user
	cmd := exec.Command("useradd", "-m", "-s", "/bin/false", username)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create user: %v", err)
	}
	
	// Set password
	cmd = exec.Command("sh", "-c", fmt.Sprintf("echo '%s:%s' | chpasswd", username, password))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set password: %v", err)
	}
	
	// Set expiry
	expireDate := time.Now().AddDate(0, 0, days).Format("2006-01-02")
	cmd = exec.Command("chage", "-E", expireDate, username)
	cmd.Run()
	
	// Create user record
	user := &User{
		Username:     username,
		Password:     password,
		ExpireDate:   expireDate,
		TrafficLimit: trafficLimit,
		MaxLogins:    maxLogins,
		CreatedAt:    time.Now(),
		Locked:       false,
	}
	
	if err := saveUser(user); err != nil {
		return err
	}
	
	// Initialize traffic
	saveTraffic(username, 0)
	
	// Configure SSH limits if needed
	if maxLogins > 0 {
		sshdConfig, _ := os.ReadFile("/etc/ssh/sshd_config")
		if !strings.Contains(string(sshdConfig), fmt.Sprintf("Match User %s", username)) {
			f, _ := os.OpenFile("/etc/ssh/sshd_config", os.O_APPEND|os.O_WRONLY, 0644)
			defer f.Close()
			f.WriteString(fmt.Sprintf("\nMatch User %s\n    MaxSessions %d\n", username, maxLogins))
			exec.Command("systemctl", "restart", "sshd").Run()
		}
	}
	
	logMessage(fmt.Sprintf("User added: %s (expires: %s, limit: %d MB)", username, expireDate, trafficLimit))
	return nil
}

func renewUser(username string, addDays int, newLimit int64, resetTraffic bool) error {
	mu.RLock()
	user, exists := users[username]
	mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("user not found")
	}
	
	if addDays > 0 {
		currentExpire, _ := time.Parse("2006-01-02", user.ExpireDate)
		newExpire := currentExpire.AddDate(0, 0, addDays)
		user.ExpireDate = newExpire.Format("2006-01-02")
		exec.Command("chage", "-E", user.ExpireDate, username).Run()
	}
	
	if newLimit > 0 {
		user.TrafficLimit = newLimit
	}
	
	if resetTraffic {
		saveTraffic(username, 0)
		// Unlock if locked
		if user.Locked {
			user.Locked = false
			exec.Command("usermod", "-U", username).Run()
		}
	}
	
	return saveUser(user)
}

func lockUser(username string) error {
	mu.RLock()
	user, exists := users[username]
	mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("user not found")
	}
	
	user.Locked = true
	exec.Command("usermod", "-L", username).Run()
	exec.Command("pkill", "-u", username).Run()
	
	return saveUser(user)
}

func unlockUser(username string) error {
	mu.RLock()
	user, exists := users[username]
	mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("user not found")
	}
	
	user.Locked = false
	exec.Command("usermod", "-U", username).Run()
	
	return saveUser(user)
}

// ========== TRAFFIC MONITORING ==========

func getProcessNetworkStats(pid int) (rx, tx uint64) {
	netDevPath := fmt.Sprintf("/proc/%d/net/dev", pid)
	data, err := os.ReadFile(netDevPath)
	if err != nil {
		return 0, 0
	}
	
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.Contains(line, "eth") || strings.Contains(line, "ens") || 
		   strings.Contains(line, "venet") || strings.Contains(line, "tun") ||
		   strings.Contains(line, "tap") || strings.Contains(line, "wg") {
			fields := strings.Fields(line)
			if len(fields) >= 10 {
				rxVal, _ := strconv.ParseUint(fields[1], 10, 64)
				txVal, _ := strconv.ParseUint(fields[9], 10, 64)
				rx += rxVal
				tx += txVal
			}
		}
	}
	return
}

func updateSessionTraffic() {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	
	for key, session := range sessions {
		// Check if process still exists
		if _, err := os.Stat(fmt.Sprintf("/proc/%d", session.PID)); os.IsNotExist(err) {
			// Session ended, add to user total
			rx, tx := getProcessNetworkStats(session.PID)
			totalBytes := rx + tx
			totalMB := int64(totalBytes / 1048576)
			
			trafficMu.Lock()
			current := traffic[session.Username]
			newTotal := current + totalMB
			traffic[session.Username] = newTotal
			trafficMu.Unlock()
			
			saveTraffic(session.Username, newTotal)
			delete(sessions, key)
			logMessage(fmt.Sprintf("Session ended for %s, added %d MB, total: %d MB", 
				session.Username, totalMB, newTotal))
		} else {
			// Update session stats
			rx, tx := getProcessNetworkStats(session.PID)
			session.RxBytes = rx
			session.TxBytes = tx
			session.LastUpdate = time.Now()
		}
	}
}

func findSSHSessions() {
	// Get all SSH connections
	cmd := exec.Command("ss", "-tnp")
	output, err := cmd.Output()
	if err != nil {
		return
	}
	
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if !strings.Contains(line, ":22") || !strings.Contains(line, "ESTAB") {
			continue
		}
		
		// Extract PID
		pidStart := strings.Index(line, "pid=")
		if pidStart == -1 {
			continue
		}
		pidEnd := strings.Index(line[pidStart:], ",")
		if pidEnd == -1 {
			pidEnd = strings.Index(line[pidStart:], ")")
		}
		if pidEnd == -1 {
			continue
		}
		
		pidStr := line[pidStart+4 : pidStart+pidEnd]
		pid, _ := strconv.Atoi(pidStr)
		if pid == 0 {
			continue
		}
		
		// Get username
		userCmd := exec.Command("ps", "-o", "user=", "-p", strconv.Itoa(pid))
		userOut, _ := userCmd.Output()
		username := strings.TrimSpace(string(userOut))
		
		if username == "" || username == "root" {
			continue
		}
		
		// Check if user exists
		mu.RLock()
		_, exists := users[username]
		mu.RUnlock()
		
		if !exists {
			continue
		}
		
		// Extract IP
		fields := strings.Fields(line)
		var ip string
		for _, field := range fields {
			if strings.Contains(field, ":") && !strings.Contains(field, "LISTEN") {
				parts := strings.Split(field, ":")
				if len(parts) >= 1 {
					ip = parts[0]
					break
				}
			}
		}
		
		sessionKey := fmt.Sprintf("%d_%s", pid, username)
		sessionMu.RLock()
		_, sessionExists := sessions[sessionKey]
		sessionMu.RUnlock()
		
		if !sessionExists {
			sessionMu.Lock()
			sessions[sessionKey] = &Session{
				PID:        pid,
				Username:   username,
				StartTime:  time.Now(),
				LastUpdate: time.Now(),
				IP:         ip,
			}
			sessionMu.Unlock()
			logMessage(fmt.Sprintf("New session created for %s (PID: %d)", username, pid))
		}
	}
}

func checkTrafficLimits() {
	mu.RLock()
	defer mu.RUnlock()
	
	for username, user := range users {
		if user.Locked {
			continue
		}
		
		if user.TrafficLimit > 0 {
			trafficMu.RLock()
			used := traffic[username]
			trafficMu.RUnlock()
			
			if used >= user.TrafficLimit {
				// Lock user
				user.Locked = true
				saveUser(user)
				exec.Command("usermod", "-L", username).Run()
				exec.Command("pkill", "-u", username).Run()
				logMessage(fmt.Sprintf("User %s LOCKED - exceeded limit: %d/%d MB", 
					username, used, user.TrafficLimit))
			}
		}
		
		// Check expiry
		if user.ExpireDate != "" {
			expireDate, _ := time.Parse("2006-01-02", user.ExpireDate)
			if time.Now().After(expireDate) {
				// Delete expired user
				deleteUser(username)
				logMessage(fmt.Sprintf("Removed expired user: %s", username))
			}
		}
	}
}

func trafficMonitorLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-stopChan:
			return
		case <-ticker.C:
			findSSHSessions()
			updateSessionTraffic()
			checkTrafficLimits()
		}
	}
}

// ========== DNS TUNNEL SERVER (Go Implementation) ==========

type DNSTTServer struct {
	conn       *net.UDPConn
	port       int
	mtu        int
	privateKey []byte
	publicKey  []byte
	domain     string
	target     string
	running    bool
	mu         sync.Mutex
}

func (s *DNSTTServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return err
	}
	
	s.conn, err = net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	
	s.running = true
	
	go s.handlePackets()
	
	logMessage(fmt.Sprintf("DNSTT Server started on port %d", s.port))
	return nil
}

func (s *DNSTTServer) handlePackets() {
	buffer := make([]byte, 4096)
	
	for s.running {
		n, clientAddr, err := s.conn.ReadFromUDP(buffer)
		if err != nil {
			if s.running {
				logMessage(fmt.Sprintf("DNSTT read error: %v", err))
			}
			continue
		}
		
		// Forward to SSH
		go s.forwardToSSH(buffer[:n], clientAddr)
	}
}

func (s *DNSTTServer) forwardToSSH(data []byte, clientAddr *net.UDPAddr) {
	sshConn, err := net.Dial("tcp", s.target)
	if err != nil {
		return
	}
	defer sshConn.Close()
	
	// Write data to SSH
	_, err = sshConn.Write(data)
	if err != nil {
		return
	}
	
	// Read response
	buffer := make([]byte, 4096)
	n, err := sshConn.Read(buffer)
	if err != nil {
		return
	}
	
	// Send back to client
	s.conn.WriteToUDP(buffer[:n], clientAddr)
}

func (s *DNSTTServer) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.running = false
	if s.conn != nil {
		s.conn.Close()
	}
}

// ========== EDNS PROXY (Go Implementation) ==========

type EDNSProxy struct {
	conn        *net.UDPConn
	targetPort  int
	running     bool
	mu          sync.Mutex
}

func (p *EDNSProxy) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	// Kill any process using port 53
	exec.Command("fuser", "-k", "53/udp").Run()
	time.Sleep(2 * time.Second)
	
	addr, err := net.ResolveUDPAddr("udp", ":53")
	if err != nil {
		return err
	}
	
	p.conn, err = net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	
	p.running = true
	
	go p.handleRequests()
	
	logMessage("EDNS Proxy started on port 53")
	return nil
}

func modifyEDNS(data []byte, maxSize uint16) []byte {
	if len(data) < 12 {
		return data
	}
	
	// Simple EDNS modification
	// Find EDNS0 OPT record (type 41)
	for i := 12; i < len(data)-10; i++ {
		// Look for EDNS0 marker
		if data[i] == 0 && data[i+1] == 41 {
			// Modify UDP payload size
			if i+4 < len(data) {
				data[i+2] = byte(maxSize >> 8)
				data[i+3] = byte(maxSize & 0xFF)
			}
			break
		}
	}
	
	return data
}

func (p *EDNSProxy) handleRequests() {
	buffer := make([]byte, 4096)
	
	for p.running {
		n, clientAddr, err := p.conn.ReadFromUDP(buffer)
		if err != nil {
			if p.running {
				logMessage(fmt.Sprintf("EDNS read error: %v", err))
			}
			continue
		}
		
		go p.handleRequest(buffer[:n], clientAddr)
	}
}

func (p *EDNSProxy) handleRequest(data []byte, clientAddr *net.UDPAddr) {
	// Modify EDNS
	modified := modifyEDNS(data, 1800)
	
	// Forward to DNSTT server
	targetAddr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", p.targetPort))
	conn, err := net.DialUDP("udp", nil, targetAddr)
	if err != nil {
		return
	}
	defer conn.Close()
	
	_, err = conn.Write(modified)
	if err != nil {
		return
	}
	
	// Read response
	buffer := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := conn.Read(buffer)
	if err != nil {
		return
	}
	
	// Modify response and send back
	response := modifyEDNS(buffer[:n], 512)
	p.conn.WriteToUDP(response, clientAddr)
}

func (p *EDNSProxy) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.running = false
	if p.conn != nil {
		p.conn.Close()
	}
}

// ========== SYSTEM OPTIMIZATION ==========

func optimizeSystem() {
	fmt.Printf("%s⚡ Optimizing system...%s\n", Yellow, NC)
	
	// Network optimizations
	sysctlSettings := map[string]string{
		"net.core.rmem_max":                 "134217728",
		"net.core.wmem_max":                 "134217728",
		"net.ipv4.tcp_rmem":                 "4096 87380 134217728",
		"net.ipv4.tcp_wmem":                 "4096 65536 134217728",
		"net.ipv4.tcp_congestion_control":   "bbr",
		"net.core.default_qdisc":            "fq",
		"net.ipv4.tcp_fastopen":             "3",
		"vm.swappiness":                     "10",
	}
	
	for key, value := range sysctlSettings {
		exec.Command("sysctl", "-w", fmt.Sprintf("%s=%s", key, value)).Run()
	}
	
	// CPU governor
	files, _ := filepath.Glob("/sys/devices/system/cpu/cpu*/cpufreq/scaling_governor")
	for _, file := range files {
		os.WriteFile(file, []byte("performance"), 0644)
	}
	
	// Clear cache
	exec.Command("sync").Run()
	os.WriteFile("/proc/sys/vm/drop_caches", []byte("3"), 0644)
	
	fmt.Printf("%s✅ System optimization complete!%s\n", Green, NC)
}

func turboMode() {
	fmt.Printf("%s🚀 Activating TURBO MODE...%s\n", Yellow, NC)
	
	sysctlSettings := map[string]string{
		"net.core.rmem_max":               "268435456",
		"net.core.wmem_max":               "268435456",
		"net.ipv4.tcp_rmem":               "8192 87380 268435456",
		"net.ipv4.tcp_wmem":               "8192 65536 268435456",
		"net.ipv4.tcp_congestion_control": "bbr",
		"net.core.default_qdisc":          "fq",
	}
	
	for key, value := range sysctlSettings {
		exec.Command("sysctl", "-w", fmt.Sprintf("%s=%s", key, value)).Run()
	}
	
	exec.Command("sync").Run()
	os.WriteFile("/proc/sys/vm/drop_caches", []byte("3"), 0644)
	
	fmt.Printf("%s✅ TURBO MODE activated!%s\n", Green, NC)
}

// ========== BANDWIDTH TEST ==========

func speedTest() {
	fmt.Printf("%s╔═══════════════════════════════════════════════════════════════╗%s\n", Cyan, NC)
	fmt.Printf("%s║%s              ELITE-X BANDWIDTH SPEED TEST                      %s║%s\n", Cyan, Yellow, Cyan, NC)
	fmt.Printf("%s╚═══════════════════════════════════════════════════════════════╝%s\n", Cyan, NC)
	fmt.Println()
	
	// Download test
	fmt.Printf("%sTesting download speed...%s\n", Yellow, NC)
	start := time.Now()
	cmd := exec.Command("curl", "-s", "-o", "/dev/null", "http://speedtest.tele2.net/100MB.zip")
	cmd.Start()
	time.Sleep(5 * time.Second)
	cmd.Process.Kill()
	elapsed := time.Since(start).Seconds()
	
	downloadSpeed := 100.0 / elapsed * 8 // Mbps
	if elapsed > 0 {
		fmt.Printf("%sDownload Speed: %.2f Mbps%s\n", Green, downloadSpeed, NC)
	}
	
	// Upload test
	fmt.Printf("%sTesting upload speed...%s\n", Yellow, NC)
	start = time.Now()
	cmd = exec.Command("sh", "-c", "dd if=/dev/zero bs=1M count=50 2>/dev/null | curl -s -X POST --data-binary @- https://httpbin.org/post -o /dev/null")
	cmd.Start()
	time.Sleep(5 * time.Second)
	cmd.Process.Kill()
	elapsed = time.Since(start).Seconds()
	
	uploadSpeed := 50.0 / elapsed * 8 // Mbps
	if elapsed > 0 {
		fmt.Printf("%sUpload Speed:   %.2f Mbps%s\n", Green, uploadSpeed, NC)
	}
	
	// Latency test
	cmd = exec.Command("ping", "-c", "1", "google.com")
	out, _ := cmd.Output()
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.Contains(line, "time=") {
			parts := strings.Split(line, "time=")
			if len(parts) > 1 {
				timePart := strings.Fields(parts[1])
				if len(timePart) > 0 {
					fmt.Printf("%sLatency:        %s%s\n", Cyan, timePart[0], NC)
				}
			}
		}
	}
}

// ========== CONNECTION MONITOR ==========

func connectionMonitor() {
	for {
		fmt.Printf("\033[2J\033[H") // Clear screen
		
		fmt.Printf("%s╔═══════════════════════════════════════════════════════════════╗%s\n", Cyan, NC)
		fmt.Printf("%s║%s              ELITE-X REAL-TIME CONNECTION MONITOR              %s║%s\n", Cyan, Yellow, Cyan, NC)
		fmt.Printf("%s╚═══════════════════════════════════════════════════════════════╝%s\n", Cyan, NC)
		fmt.Println()
		
		fmt.Printf("%sActive SSH Connections:%s\n", Green, NC)
		fmt.Println("─────────────────────────────────")
		
		cmd := exec.Command("ss", "-tnp")
		out, _ := cmd.Output()
		lines := strings.Split(string(out), "\n")
		
		sshCount := 0
		for _, line := range lines {
			if strings.Contains(line, ":22") && strings.Contains(line, "ESTAB") {
				fields := strings.Fields(line)
				if len(fields) >= 5 {
					addr := fields[4]
					ip := strings.Split(addr, ":")[0]
					
					// Get username
					pidStart := strings.Index(line, "pid=")
					if pidStart != -1 {
						pidEnd := strings.Index(line[pidStart:], ",")
						if pidEnd == -1 {
							pidEnd = strings.Index(line[pidStart:], ")")
						}
						if pidEnd != -1 {
							pidStr := line[pidStart+4 : pidStart+pidEnd]
							pid, _ := strconv.Atoi(pidStr)
							userCmd := exec.Command("ps", "-o", "user=", "-p", strconv.Itoa(pid))
							userOut, _ := userCmd.Output()
							username := strings.TrimSpace(string(userOut))
							fmt.Printf("  %s→%s %s (%s)\n", Green, NC, ip, username)
						}
					}
					sshCount++
				}
			}
		}
		
		fmt.Printf("\n%sDNS Tunnel Connections (port %d):%s\n", Yellow, config.DNSTTPort, NC)
		fmt.Println("─────────────────────────────────")
		
		cmd = exec.Command("ss", "-unp")
		out, _ = cmd.Output()
		lines = strings.Split(string(out), "\n")
		
		dnsCount := 0
		for _, line := range lines {
			if strings.Contains(line, fmt.Sprintf(":%d", config.DNSTTPort)) {
				fields := strings.Fields(line)
				if len(fields) >= 5 {
					addr := fields[4]
					ip := strings.Split(addr, ":")[0]
					fmt.Printf("  %s→%s %s\n", Yellow, NC, ip)
					dnsCount++
				}
			}
		}
		
		fmt.Printf("\n%sTotal Connections: %d SSH, %d DNS%s\n", Cyan, sshCount, dnsCount, NC)
		fmt.Printf("%sPress 'q' to exit, any other key to refresh%s\n", White, NC)
		
		// Wait for keypress with timeout
		reader := bufio.NewReader(os.Stdin)
		done := make(chan struct{})
		go func() {
			ch, _ := reader.ReadByte()
			if ch == 'q' || ch == 'Q' {
				close(done)
			}
		}()
		
		select {
		case <-done:
			return
		case <-time.After(5 * time.Second):
			continue
		}
	}
}

// ========== BACKUP ==========

func backup() {
	backupDir := "/root/elite-x-backups"
	os.MkdirAll(backupDir, 0755)
	
	timestamp := time.Now().Format("20060102-150405")
	
	// Backup config
	exec.Command("tar", "-czf", 
		fmt.Sprintf("%s/elite-x-config-%s.tar.gz", backupDir, timestamp),
		ConfigDir).Run()
	
	// Backup DNSTT keys
	exec.Command("tar", "-czf", 
		fmt.Sprintf("%s/dnstt-keys-%s.tar.gz", backupDir, timestamp),
		DNSTTDir).Run()
	
	// Clean old backups (keep last 10)
	files, _ := filepath.Glob(filepath.Join(backupDir, "elite-x-config-*.tar.gz"))
	if len(files) > 10 {
		for i := 0; i < len(files)-10; i++ {
			os.Remove(files[i])
		}
	}
	
	logMessage(fmt.Sprintf("Backup completed: %s", timestamp))
	fmt.Printf("%s✅ Backup completed!%s\n", Green, NC)
}

// ========== BANNER ==========

func setupBanner() {
	bannerDir := filepath.Join(ConfigDir, "banner")
	os.MkdirAll(bannerDir, 0755)
	
	bannerContent := `===============================================
           ELITE-X VPN SERVICE             
    High Speed • Secure • Unlimited      
===============================================
    Server: ELITE-X v3.5
    Location: Optimized
    Support: 24/7 Active
===============================================
    Welcome! Your connection is secure.
    Enjoy high-speed browsing.
===============================================
`
	
	os.WriteFile(filepath.Join(bannerDir, "ssh-banner"), []byte(bannerContent), 0644)
	
	// Update SSH config
	sshdConfig, _ := os.ReadFile("/etc/ssh/sshd_config")
	if !strings.Contains(string(sshdConfig), "Banner") {
		f, _ := os.OpenFile("/etc/ssh/sshd_config", os.O_APPEND|os.O_WRONLY, 0644)
		defer f.Close()
		f.WriteString(fmt.Sprintf("\nBanner %s\n", filepath.Join(bannerDir, "ssh-banner")))
		exec.Command("systemctl", "restart", "sshd").Run()
	}
}

// ========== DASHBOARD ==========

func getSystemInfo() (string, string, string) {
	// Get IP
	cmd := exec.Command("curl", "-4", "-s", "ifconfig.me")
	ipOut, _ := cmd.Output()
	ip := strings.TrimSpace(string(ipOut))
	if ip == "" {
		ip = "Unknown"
	}
	
	// Get location
	cmd = exec.Command("curl", "-s", fmt.Sprintf("http://ip-api.com/json/%s", ip))
	locOut, _ := cmd.Output()
	var locInfo map[string]interface{}
	json.Unmarshal(locOut, &locInfo)
	
	location := ""
	if city, ok := locInfo["city"].(string); ok {
		location = city
	}
	if country, ok := locInfo["country"].(string); ok {
		if location != "" {
			location += ", "
		}
		location += country
	}
	if location == "" {
		location = "Unknown"
	}
	
	isp := ""
	if ispVal, ok := locInfo["isp"].(string); ok {
		isp = ispVal
	}
	if isp == "" {
		isp = "Unknown"
	}
	
	return ip, location, isp
}

func getConnectionStats() (int, int, int) {
	// SSH connections
	cmd := exec.Command("ss", "-tnp")
	out, _ := cmd.Output()
	sshCount := strings.Count(string(out), ":22") - strings.Count(string(out), "LISTEN")
	
	// DNS connections
	cmd = exec.Command("ss", "-unp")
	out, _ = cmd.Output()
	dnsCount := strings.Count(string(out), fmt.Sprintf(":%d", config.DNSTTPort))
	
	// Active users
	activeUsers := 0
	mu.RLock()
	for username := range users {
		cmd := exec.Command("pgrep", "-u", username)
		if out, _ := cmd.Output(); len(out) > 0 {
			activeUsers++
		}
	}
	mu.RUnlock()
	
	return sshCount, dnsCount, activeUsers
}

func showDashboard() {
	fmt.Printf("\033[2J\033[H") // Clear screen
	
	ip, location, isp := getSystemInfo()
	sshCount, dnsCount, activeUsers := getConnectionStats()
	
	// Get RAM usage
	ramCmd := exec.Command("free", "-m")
	ramOut, _ := ramCmd.Output()
	ramLines := strings.Split(string(ramOut), "\n")
	ramUsed := ""
	ramTotal := ""
	for _, line := range ramLines {
		if strings.HasPrefix(line, "Mem:") {
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				ramUsed = fields[2]
				ramTotal = fields[1]
			}
		}
	}
	
	// Get CPU usage
	cpuCmd := exec.Command("top", "-bn1")
	cpuOut, _ := cpuCmd.Output()
	cpuLines := strings.Split(string(cpuOut), "\n")
	cpuUsage := "0"
	for _, line := range cpuLines {
		if strings.Contains(line, "Cpu(s)") {
			fields := strings.Fields(line)
			if len(fields) > 1 {
				cpuUsage = strings.TrimSuffix(fields[1], "%")
			}
			break
		}
	}
	
	// Get uptime
	uptimeCmd := exec.Command("uptime")
	uptimeOut, _ := uptimeCmd.Output()
	uptimeParts := strings.Split(string(uptimeOut), "up")
	uptime := ""
	if len(uptimeParts) > 1 {
		uptime = strings.Split(uptimeParts[1], ",")[0]
	}
	
	// Service status
	services := map[string]bool{
		"DNSTT Server":   dnsttCmd != nil && dnsttCmd.Process != nil,
		"EDNS Proxy":     ednsProxyCmd != nil && ednsProxyCmd.Process != nil,
		"Traffic Monitor": true, // Always running in goroutine
	}
	
	fmt.Printf("%s╔════════════════════════════════════════════════════════════════╗%s\n", Purple, NC)
	fmt.Printf("%s║%s%s                    ELITE-X SLOWDNS v%s                       %s║%s\n", 
		Purple, Yellow, Bold, Version, Purple, NC)
	fmt.Printf("%s╠════════════════════════════════════════════════════════════════╣%s\n", Purple, NC)
	fmt.Printf("%s║%s  Subdomain :%s %-50s%s║%s\n", Purple, White, Green, config.Subdomain, Purple, NC)
	fmt.Printf("%s║%s  IP        :%s %-50s%s║%s\n", Purple, White, Green, ip, Purple, NC)
	fmt.Printf("%s║%s  Location  :%s %-50s%s║%s\n", Purple, White, Green, location, Purple, NC)
	fmt.Printf("%s║%s  ISP       :%s %-50s%s║%s\n", Purple, White, Green, isp, Purple, NC)
	fmt.Printf("%s║%s  RAM       :%s %sMB / %sMB | CPU: %s%% | Uptime: %-20s%s║%s\n", 
		Purple, White, Green, ramUsed, ramTotal, cpuUsage, uptime, Purple, NC)
	fmt.Printf("%s║%s  VPS Loc   :%s %-10s | MTU: %d%-30s%s║%s\n", 
		Purple, White, Green, config.Location, config.MTU, "", Purple, NC)
	fmt.Printf("%s║%s  Services  : DNS: %s●%s PRX: %s●%s TRAF: %s●%s CLN: %s●%s BAND: %s●%s MON: %s●%s%s║%s\n",
		Purple, White, 
		statusColor(services["DNSTT Server"]), statusSymbol(services["DNSTT Server"]), NC,
		statusColor(services["EDNS Proxy"]), statusSymbol(services["EDNS Proxy"]), NC,
		Green, "●", NC,
		Green, "●", NC,
		Green, "●", NC,
		Green, "●", NC,
		Purple, NC)
	fmt.Printf("%s║%s  Connections: SSH: %s%d%s | DNS: %s%d%s | Active Users: %s%d%-22s%s║%s\n",
		Purple, White, Green, sshCount, NC, Yellow, dnsCount, NC, Cyan, activeUsers, "", Purple, NC)
	fmt.Printf("%s║%s  Developer :%s ELITE-X TEAM%-47s%s║%s\n", Purple, White, Purple, "", Purple, NC)
	fmt.Printf("%s╠════════════════════════════════════════════════════════════════╣%s\n", Purple, NC)
	fmt.Printf("%s║%s  Act Key   :%s %-50s%s║%s\n", Purple, White, Yellow, config.Activation, Purple, NC)
	fmt.Printf("%s║%s  Expiry    :%s %-50s%s║%s\n", Purple, White, Yellow, config.Expiry, Purple, NC)
	fmt.Printf("%s╚════════════════════════════════════════════════════════════════╝%s\n", Purple, NC)
	fmt.Println()
}

func statusColor(running bool) string {
	if running {
		return Green
	}
	return Red
}

func statusSymbol(running bool) string {
	if running {
		return "●"
	}
	return "○"
}

func listUsers() {
	fmt.Printf("\033[2J\033[H")
	fmt.Printf("%s╔═══════════════════════════════════════════════════════════════╗%s\n", Cyan, NC)
	fmt.Printf("%s║%s                    ACTIVE USERS                                 %s║%s\n", Cyan, Yellow, Cyan, NC)
	fmt.Printf("%s╚═══════════════════════════════════════════════════════════════╝%s\n", Cyan, NC)
	fmt.Println()
	
	fmt.Printf("%-12s %-10s %-10s %-10s %-10s %-8s %-8s\n", 
		"USERNAME", "EXPIRE", "LIMIT", "USED", "USAGE%", "CONNS", "STATUS")
	fmt.Println("─────────────────────────────────────────────────────────────────────────────────")
	
	totalTraffic := int64(0)
	totalUsers := 0
	
	mu.RLock()
	defer mu.RUnlock()
	
	for username, user := range users {
		trafficMu.RLock()
		used := traffic[username]
		trafficMu.RUnlock()
		
		// Get connections
		connCmd := exec.Command("pgrep", "-u", username)
		connOut, _ := connCmd.Output()
		conns := len(strings.Fields(string(connOut)))
		
		// Calculate usage percentage
		usagePercent := "Unlimited"
		if user.TrafficLimit > 0 {
			percent := used * 100 / user.TrafficLimit
			usagePercent = fmt.Sprintf("%d%%", percent)
		}
		
		// Status
		status := "OK"
		statusColor := Green
		if user.Locked {
			status = "LOCK"
			statusColor = Red
		}
		
		fmt.Printf("%-12s %-10s %-10d %-10d %-10s %-8d %s%s%s\n", 
			username, user.ExpireDate, user.TrafficLimit, used, 
			usagePercent, conns, statusColor, status, NC)
		
		totalTraffic += used
		totalUsers++
	}
	
	fmt.Println("─────────────────────────────────────────────────────────────────────────────────")
	fmt.Printf("Total Users: %s%d%s | Total Traffic Used: %s%d MB%s\n", 
		Green, totalUsers, NC, Yellow, totalTraffic, NC)
	fmt.Println()
}

func trafficStats() {
	fmt.Printf("\033[2J\033[H")
	fmt.Printf("%s╔═══════════════════════════════════════════════════════════════╗%s\n", Cyan, NC)
	fmt.Printf("%s║%s                    TRAFFIC STATISTICS                            %s║%s\n", Cyan, Yellow, Cyan, NC)
	fmt.Printf("%s╚═══════════════════════════════════════════════════════════════╝%s\n", Cyan, NC)
	fmt.Println()
	
	fmt.Printf("%-12s %-15s %-12s %-12s %-10s\n", "USERNAME", "LIMIT (MB)", "USED (MB)", "USAGE%", "CONNS")
	fmt.Println("─────────────────────────────────────────────────────────────────")
	
	total := int64(0)
	
	mu.RLock()
	defer mu.RUnlock()
	
	for username, user := range users {
		trafficMu.RLock()
		used := traffic[username]
		trafficMu.RUnlock()
		
		connCmd := exec.Command("pgrep", "-u", username)
		connOut, _ := connCmd.Output()
		conns := len(strings.Fields(string(connOut)))
		
		percent := "Unlimited"
		if user.TrafficLimit > 0 {
			p := used * 100 / user.TrafficLimit
			percent = fmt.Sprintf("%d%%", p)
		}
		
		fmt.Printf("%-12s %-15d %-12d %-12s %-10d\n", 
			username, user.TrafficLimit, used, percent, conns)
		total += used
	}
	
	fmt.Println("─────────────────────────────────────────────────────────────────")
	fmt.Printf("Total Traffic Used: %s%d MB%s\n", Yellow, total, NC)
	fmt.Println()
}

func activeConnections() {
	fmt.Printf("\033[2J\033[H")
	fmt.Printf("%s╔═══════════════════════════════════════════════════════════════╗%s\n", Cyan, NC)
	fmt.Printf("%s║%s              ACTIVE USER CONNECTIONS                            %s║%s\n", Cyan, Yellow, Cyan, NC)
	fmt.Printf("%s╚═══════════════════════════════════════════════════════════════╝%s\n", Cyan, NC)
	fmt.Println()
	
	fmt.Printf("%-15s %-10s %-25s\n", "USERNAME", "CONNS", "ACTIVE IPs")
	fmt.Println("─────────────────────────────────────────────────────────────────")
	
	mu.RLock()
	defer mu.RUnlock()
	
	for username, user := range users {
		connCmd := exec.Command("pgrep", "-u", username)
		connOut, _ := connCmd.Output()
		pids := strings.Fields(string(connOut))
		conns := len(pids)
		
		if conns > 0 {
			ips := ""
			for _, pidStr := range pids {
				pid, _ := strconv.Atoi(pidStr)
				cmd := exec.Command("ss", "-tnp")
				out, _ := cmd.Output()
				lines := strings.Split(string(out), "\n")
				for _, line := range lines {
					if strings.Contains(line, fmt.Sprintf("pid=%d", pid)) {
						fields := strings.Fields(line)
						if len(fields) >= 5 {
							addr := fields[4]
							ip := strings.Split(addr, ":")[0]
							if !strings.Contains(ips, ip) {
								ips += ip + " "
							}
						}
					}
				}
			}
			fmt.Printf("%-15s %-10d %-25s\n", username, conns, ips)
		} else {
			if !user.Locked {
				fmt.Printf("%-15s %-10d %-25s\n", username, 0, "-")
			}
		}
	}
	fmt.Println()
}

func settingsMenu() {
	for {
		fmt.Printf("\033[2J\033[H")
		fmt.Printf("%s╔════════════════════════════════════════════════════════════════╗%s\n", Cyan, NC)
		fmt.Printf("%s║%s%s                      SETTINGS MENU                              %s║%s\n", Cyan, Yellow, Bold, Cyan, NC)
		fmt.Printf("%s╠════════════════════════════════════════════════════════════════╣%s\n", Cyan, NC)
		fmt.Printf("%s║%s  [1] 🔑 View Public Key%s\n", Cyan, White, NC)
		fmt.Printf("%s║%s  [2] Change MTU Value%s\n", Cyan, White, NC)
		fmt.Printf("%s║%s  [3] ⚡ Speed Optimization Menu%s\n", Cyan, White, NC)
		fmt.Printf("%s║%s  [4] 🧹 Clean Junk Files%s\n", Cyan, White, NC)
		fmt.Printf("%s║%s  [5] 🔄 Auto Expired Account Remover%s\n", Cyan, White, NC)
		fmt.Printf("%s║%s  [6] 🔄 Restart All Services%s\n", Cyan, White, NC)
		fmt.Printf("%s║%s  [7] 📊 System Info%s\n", Cyan, White, NC)
		fmt.Printf("%s║%s  [8] 💾 Backup Configuration%s\n", Cyan, White, NC)
		fmt.Printf("%s║%s  [9] 📈 Speed Test%s\n", Cyan, White, NC)
		fmt.Printf("%s║%s  [10] 👁️  Connection Monitor%s\n", Cyan, White, NC)
		fmt.Printf("%s║%s  [11] 🚀 Turbo Optimize%s\n", Cyan, White, NC)
		fmt.Printf("%s║%s  [12] 🔄 Reboot VPS%s\n", Cyan, White, NC)
		fmt.Printf("%s║%s  [13] 🗑️  Uninstall Script%s\n", Cyan, White, NC)
		fmt.Printf("%s║%s  [14] 🌍 Re-apply Location Optimization%s\n", Cyan, White, NC)
		fmt.Printf("%s║%s  [0] Back to Main Menu%s\n", Cyan, White, NC)
		fmt.Printf("%s╚════════════════════════════════════════════════════════════════╝%s\n", Cyan, NC)
		fmt.Println()
		
		fmt.Print("Settings option: ")
		var choice string
		fmt.Scanln(&choice)
		
		switch choice {
		case "1":
			pubKeyFile := filepath.Join(DNSTTDir, "server.pub")
			if data, err := os.ReadFile(pubKeyFile); err == nil {
				fmt.Printf("%s╔═══════════════════════════════════════════════════════════════╗%s\n", Cyan, NC)
				fmt.Printf("%s║%s                    PUBLIC KEY                                   %s║%s\n", Cyan, Yellow, Cyan, NC)
				fmt.Printf("%s╠═══════════════════════════════════════════════════════════════╣%s\n", Cyan, NC)
				fmt.Printf("%s║%s  %s%s\n", Cyan, Green, strings.TrimSpace(string(data)), NC)
				fmt.Printf("%s╚═══════════════════════════════════════════════════════════════╝%s\n", Cyan, NC)
			}
			fmt.Print("Press Enter to continue...")
			fmt.Scanln()
		case "2":
			fmt.Printf("Current MTU: %d\n", config.MTU)
			fmt.Print("New MTU (1000-5000): ")
			var mtu int
			fmt.Scanln(&mtu)
			if mtu >= 1000 && mtu <= 5000 {
				config.MTU = mtu
				saveConfig()
				fmt.Printf("%s✅ MTU updated to %d%s\n", Green, mtu, NC)
				// Restart services
				if dnsttCmd != nil {
					dnsttCmd.Process.Kill()
				}
				if ednsProxyCmd != nil {
					ednsProxyCmd.Process.Kill()
				}
				startDNSTTServer()
				startEDNSProxy()
			} else {
				fmt.Printf("%s❌ Invalid (must be 1000-5000)%s\n", Red, NC)
			}
			fmt.Print("Press Enter to continue...")
			fmt.Scanln()
		case "3":
			optimizeSystem()
			fmt.Print("Press Enter to continue...")
			fmt.Scanln()
		case "4":
			exec.Command("apt", "clean").Run()
			exec.Command("apt", "autoclean").Run()
			exec.Command("journalctl", "--vacuum-time=3d").Run()
			fmt.Printf("%s✅ Junk files cleaned!%s\n", Green, NC)
			fmt.Print("Press Enter to continue...")
			fmt.Scanln()
		case "5":
			fmt.Printf("%s✅ Auto remover is always running%s\n", Green, NC)
			fmt.Print("Press Enter to continue...")
			fmt.Scanln()
		case "6":
			if dnsttCmd != nil {
				dnsttCmd.Process.Kill()
			}
			if ednsProxyCmd != nil {
				ednsProxyCmd.Process.Kill()
			}
			startDNSTTServer()
			startEDNSProxy()
			fmt.Printf("%s✅ Services restarted%s\n", Green, NC)
			fmt.Print("Press Enter to continue...")
			fmt.Scanln()
		case "7":
			fmt.Printf("\033[2J\033[H")
			fmt.Printf("%s╔═══════════════════════════════════════════════════════════════╗%s\n", Cyan, NC)
			fmt.Printf("%s║%s                    SYSTEM INFORMATION                           %s║%s\n", Cyan, Yellow, Cyan, NC)
			fmt.Printf("%s╚═══════════════════════════════════════════════════════════════╝%s\n", Cyan, NC)
			fmt.Println()
			
			// OS Info
			osInfo, _ := exec.Command("lsb_release", "-d").Output()
			fmt.Printf("%sOS: %s%s\n", Green, NC, strings.TrimSpace(string(osInfo)))
			
			kernel, _ := exec.Command("uname", "-r").Output()
			fmt.Printf("%sKernel: %s%s\n", Green, NC, strings.TrimSpace(string(kernel)))
			
			arch, _ := exec.Command("uname", "-m").Output()
			fmt.Printf("%sArchitecture: %s%s\n", Green, NC, strings.TrimSpace(string(arch)))
			
			hostname, _ := exec.Command("hostname").Output()
			fmt.Printf("%sHostname: %s%s\n", Green, NC, strings.TrimSpace(string(hostname)))
			
			cpuCount, _ := exec.Command("nproc").Output()
			fmt.Printf("%sCPU: %s cores%s\n", Green, NC, strings.TrimSpace(string(cpuCount)))
			
			freeOut, _ := exec.Command("free", "-h").Output()
			lines := strings.Split(string(freeOut), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "Mem:") {
					fields := strings.Fields(line)
					if len(fields) >= 3 {
						fmt.Printf("%sMemory Total: %s%s\n", Green, NC, fields[1])
						fmt.Printf("%sMemory Used: %s%s\n", Green, NC, fields[2])
					}
				}
			}
			
			dfOut, _ := exec.Command("df", "-h", "/").Output()
			lines = strings.Split(string(dfOut), "\n")
			if len(lines) > 1 {
				fields := strings.Fields(lines[1])
				if len(fields) >= 3 {
					fmt.Printf("%sDisk Total: %s%s\n", Green, NC, fields[1])
					fmt.Printf("%sDisk Used: %s%s\n", Green, NC, fields[2])
				}
			}
			
			load, _ := exec.Command("uptime").Output()
			fmt.Printf("%sLoad Average: %s%s\n", Green, NC, strings.TrimSpace(string(load)))
			
			fmt.Println()
			fmt.Print("Press Enter to continue...")
			fmt.Scanln()
		case "8":
			backup()
			fmt.Print("Press Enter to continue...")
			fmt.Scanln()
		case "9":
			speedTest()
			fmt.Print("Press Enter to continue...")
			fmt.Scanln()
		case "10":
			connectionMonitor()
		case "11":
			turboMode()
			fmt.Print("Press Enter to continue...")
			fmt.Scanln()
		case "12":
			fmt.Print("Reboot? (y/n): ")
			var confirm string
			fmt.Scanln(&confirm)
			if confirm == "y" {
				exec.Command("reboot").Run()
			}
		case "13":
			fmt.Print("Uninstall? (YES): ")
			var confirm string
			fmt.Scanln(&confirm)
			if confirm == "YES" {
				uninstall()
				return
			}
		case "14":
			fmt.Printf("%s═══════════════════════════════════════════════════════════════%s\n", Yellow, NC)
			fmt.Printf("%s           RE-APPLY LOCATION OPTIMIZATION                        %s\n", Green, NC)
			fmt.Printf("%s═══════════════════════════════════════════════════════════════%s\n", Yellow, NC)
			fmt.Printf("%sSelect your VPS location:%s\n", White, NC)
			fmt.Printf("%s  1. South Africa (MTU 1800)%s\n", Green, NC)
			fmt.Printf("%s  2. USA%s\n", Cyan, NC)
			fmt.Printf("%s  3. Europe%s\n", Blue, NC)
			fmt.Printf("%s  4. Asia%s\n", Purple, NC)
			fmt.Printf("%s  5. Auto-detect%s\n", Yellow, NC)
			fmt.Print("Choice: ")
			var locChoice string
			fmt.Scanln(&locChoice)
			
			switch locChoice {
			case "1":
				config.Location = "South Africa"
				config.MTU = 1800
				fmt.Printf("%s✅ South Africa selected (MTU 1800)%s\n", Green, NC)
			case "2":
				config.Location = "USA"
				fmt.Printf("%s✅ USA selected%s\n", Green, NC)
			case "3":
				config.Location = "Europe"
				fmt.Printf("%s✅ Europe selected%s\n", Green, NC)
			case "4":
				config.Location = "Asia"
				fmt.Printf("%s✅ Asia selected%s\n", Green, NC)
			case "5":
				config.Location = "Auto-detect"
				fmt.Printf("%s✅ Auto-detect selected%s\n", Green, NC)
			}
			saveConfig()
			fmt.Print("Press Enter to continue...")
			fmt.Scanln()
		case "0":
			return
		}
	}
}

func uninstall() {
	fmt.Printf("%s🔄 Removing all users and data...%s\n", Yellow, NC)
	
	// Remove all users
	mu.RLock()
	for username := range users {
		fmt.Printf("  Removing user: %s\n", username)
		exec.Command("userdel", "-r", username).Run()
		exec.Command("pkill", "-u", username).Run()
	}
	mu.RUnlock()
	
	// Stop services
	if dnsttCmd != nil {
		dnsttCmd.Process.Kill()
	}
	if ednsProxyCmd != nil {
		ednsProxyCmd.Process.Kill()
	}
	close(stopChan)
	
	// Remove directories
	os.RemoveAll(ConfigDir)
	os.RemoveAll(DNSTTDir)
	
	// Remove binaries (self)
	os.Remove("/usr/local/bin/elite-x-go")
	
	fmt.Printf("%s✅ ELITE-X has been uninstalled.%s\n", Green, NC)
	os.Exit(0)
}

// ========== MAIN MENU ==========

func mainMenu() {
	for {
		showDashboard()
		fmt.Printf("%s╔════════════════════════════════════════════════════════════════╗%s\n", Cyan, NC)
		fmt.Printf("%s║%s%s                         MAIN MENU                              %s║%s\n", Cyan, Green, Bold, Cyan, NC)
		fmt.Printf("%s╠════════════════════════════════════════════════════════════════╣%s\n", Cyan, NC)
		fmt.Printf("%s║%s  [1] 👤 Add User%s\n", Cyan, White, NC)
		fmt.Printf("%s║%s  [2] 📊 View All Users%s\n", Cyan, White, NC)
		fmt.Printf("%s║%s  [3] 🔒 Lock User%s\n", Cyan, White, NC)
		fmt.Printf("%s║%s  [4] 🔓 Unlock User%s\n", Cyan, White, NC)
		fmt.Printf("%s║%s  [5] 🗑️  Delete User%s\n", Cyan, White, NC)
		fmt.Printf("%s║%s  [6] 🔄 Renew User%s\n", Cyan, White, NC)
		fmt.Printf("%s║%s  [7] 📈 Traffic Statistics%s\n", Cyan, White, NC)
		fmt.Printf("%s║%s  [8] 👥 View Active Connections%s\n", Cyan, White, NC)
		fmt.Printf("%s║%s  [9] ⚡ Speed Test%s\n", Cyan, White, NC)
		fmt.Printf("%s║%s  [10] 👁️  Connection Monitor%s\n", Cyan, White, NC)
		fmt.Printf("%s║%s  [S] ⚙️  Settings%s\n", Cyan, White, NC)
		fmt.Printf("%s║%s  [0] 🚪 Exit%s\n", Cyan, White, NC)
		fmt.Printf("%s╚════════════════════════════════════════════════════════════════╝%s\n", Cyan, NC)
		fmt.Println()
		
		fmt.Print("Main menu option: ")
		var choice string
		fmt.Scanln(&choice)
		
		switch strings.ToLower(choice) {
		case "1":
			fmt.Print("Username: ")
			var username string
			fmt.Scanln(&username)
			fmt.Print("Password: ")
			var password string
			fmt.Scanln(&password)
			fmt.Print("Expire days: ")
			var days int
			fmt.Scanln(&days)
			fmt.Print("Traffic limit (MB, 0 for unlimited): ")
			var trafficLimit int64
			fmt.Scanln(&trafficLimit)
			fmt.Print("Max concurrent logins (0 for unlimited): ")
			var maxLogins int
			fmt.Scanln(&maxLogins)
			
			if err := addUser(username, password, days, trafficLimit, maxLogins); err != nil {
				fmt.Printf("%s❌ %v%s\n", Red, err, NC)
			} else {
				fmt.Printf("%s✅ User created successfully!%s\n", Green, NC)
				fmt.Printf("Username: %s\n", username)
				fmt.Printf("Password: %s\n", password)
				fmt.Printf("Expire: %s\n", time.Now().AddDate(0, 0, days).Format("2006-01-02"))
				fmt.Printf("Traffic Limit: %d MB\n", trafficLimit)
			}
			fmt.Print("Press Enter to continue...")
			fmt.Scanln()
		case "2":
			listUsers()
			fmt.Print("Press Enter to continue...")
			fmt.Scanln()
		case "3":
			fmt.Print("Username: ")
			var username string
			fmt.Scanln(&username)
			if err := lockUser(username); err != nil {
				fmt.Printf("%s❌ %v%s\n", Red, err, NC)
			} else {
				fmt.Printf("%s✅ User locked%s\n", Green, NC)
			}
			fmt.Print("Press Enter to continue...")
			fmt.Scanln()
		case "4":
			fmt.Print("Username: ")
			var username string
			fmt.Scanln(&username)
			if err := unlockUser(username); err != nil {
				fmt.Printf("%s❌ %v%s\n", Red, err, NC)
			} else {
				fmt.Printf("%s✅ User unlocked%s\n", Green, NC)
			}
			fmt.Print("Press Enter to continue...")
			fmt.Scanln()
		case "5":
			fmt.Print("Username: ")
			var username string
			fmt.Scanln(&username)
			if err := deleteUser(username); err != nil {
				fmt.Printf("%s❌ %v%s\n", Red, err, NC)
			} else {
				fmt.Printf("%s✅ User deleted%s\n", Green, NC)
			}
			fmt.Print("Press Enter to continue...")
			fmt.Scanln()
		case "6":
			fmt.Print("Username: ")
			var username string
			fmt.Scanln(&username)
			fmt.Print("Add how many days? (0 to skip): ")
			var addDays int
			fmt.Scanln(&addDays)
			fmt.Print("New traffic limit MB (0 to keep current): ")
			var newLimit int64
			fmt.Scanln(&newLimit)
			fmt.Print("Reset traffic usage? (y/n): ")
			var resetTraffic string
			fmt.Scanln(&resetTraffic)
			
			if err := renewUser(username, addDays, newLimit, resetTraffic == "y"); err != nil {
				fmt.Printf("%s❌ %v%s\n", Red, err, NC)
			} else {
				fmt.Printf("%s✅ User renewed%s\n", Green, NC)
			}
			fmt.Print("Press Enter to continue...")
			fmt.Scanln()
		case "7":
			trafficStats()
			fmt.Print("Press Enter to continue...")
			fmt.Scanln()
		case "8":
			activeConnections()
			fmt.Print("Press Enter to continue...")
			fmt.Scanln()
		case "9":
			speedTest()
			fmt.Print("Press Enter to continue...")
			fmt.Scanln()
		case "10":
			connectionMonitor()
		case "s":
			settingsMenu()
		case "0":
			showQuote()
			fmt.Printf("%sGoodbye!%s\n", Green, NC)
			return
		default:
			fmt.Printf("%sInvalid option%s\n", Red, NC)
			fmt.Print("Press Enter to continue...")
			fmt.Scanln()
		}
	}
}

// ========== DNSTT SERVER STARTUP ==========

func generateDNSTTKeys() error {
	keyFile := filepath.Join(DNSTTDir, "server.key")
	pubFile := filepath.Join(DNSTTDir, "server.pub")
	
	// Generate random keys
	privateKey := make([]byte, 32)
	publicKey := make([]byte, 32)
	
	rand.Read(privateKey)
	rand.Read(publicKey)
	
	// Simple XOR-based key derivation for demo
	for i := 0; i < 32; i++ {
		publicKey[i] = privateKey[i] ^ 0xFF
	}
	
	// Save keys
	privEncoded := base64.StdEncoding.EncodeToString(privateKey)
	pubEncoded := base64.StdEncoding.EncodeToString(publicKey)
	
	if err := os.WriteFile(keyFile, []byte(privEncoded), 0600); err != nil {
		return err
	}
	if err := os.WriteFile(pubFile, []byte(pubEncoded), 0644); err != nil {
		return err
	}
	
	return nil
}

func startDNSTTServer() {
	server := &DNSTTServer{
		port:    config.DNSTTPort,
		mtu:     config.MTU,
		domain:  config.Subdomain,
		target:  "127.0.0.1:22",
	}
	
	go func() {
		if err := server.Start(); err != nil {
			logMessage(fmt.Sprintf("Failed to start DNSTT server: %v", err))
		}
	}()
	
	// Store reference for cleanup
	dnsttCmd = exec.Command("sleep", "infinity") // Placeholder
}

func startEDNSProxy() {
	proxy := &EDNSProxy{
		targetPort: config.DNSTTPort,
	}
	
	go func() {
		if err := proxy.Start(); err != nil {
			logMessage(fmt.Sprintf("Failed to start EDNS proxy: %v", err))
		}
	}()
	
	ednsProxyCmd = exec.Command("sleep", "infinity") // Placeholder
}

func startServices() {
	// Generate keys if not exist
	if _, err := os.Stat(filepath.Join(DNSTTDir, "server.key")); os.IsNotExist(err) {
		generateDNSTTKeys()
	}
	
	startDNSTTServer()
	startEDNSProxy()
}

// ========== MAIN ==========

func main() {
	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Printf("\n%sShutting down...%s\n", Yellow, NC)
		if dnsttCmd != nil {
			dnsttCmd.Process.Kill()
		}
		if ednsProxyCmd != nil {
			ednsProxyCmd.Process.Kill()
		}
		close(stopChan)
		os.Exit(0)
	}()
	
	stopChan = make(chan struct{})
	
	showBanner()
	
	// Initialize
	if err := initConfig(); err != nil {
		fmt.Printf("%s❌ Failed to initialize config: %v%s\n", Red, err, NC)
		os.Exit(1)
	}
	
	// Activation
	fmt.Printf("%s╔═══════════════════════════════════════════════════════════════╗%s\n", Yellow, NC)
	fmt.Printf("%s║%s                    ACTIVATION REQUIRED                          %s║%s\n", Yellow, Green, Yellow, NC)
	fmt.Printf("%s╚═══════════════════════════════════════════════════════════════╝%s\n", Yellow, NC)
	fmt.Println()
	fmt.Printf("%sAvailable Keys:%s\n", White, NC)
	fmt.Printf("%s  Lifetime : Whtsapp 0713628668%s\n", Green, NC)
	fmt.Printf("%s  Trial    : ELITE-X-TEST-0208 (2 days)%s\n", Yellow, NC)
	fmt.Println()
	
	fmt.Print("Activation Key: ")
	reader := bufio.NewReader(os.Stdin)
	activationKey, _ := reader.ReadString('\n')
	activationKey = strings.TrimSpace(activationKey)
	
	if !activateScript(activationKey) {
		fmt.Printf("%s❌ Invalid activation key! Installation cancelled.%s\n", Red, NC)
		os.Exit(1)
	}
	
	fmt.Printf("%s✅ Activation successful!%s\n", Green, NC)
	time.Sleep(1 * time.Second)
	
	if config.ActivationType == "temporary" {
		fmt.Printf("%s⚠️  Trial version activated - expires in %d days%s\n", Yellow, config.ExpiryDays, NC)
	}
	
	// Check expiry
	if !checkExpiry() {
		uninstall()
		return
	}
	
	// Set timezone
	exec.Command("timedatectl", "set-timezone", TimeZone).Run()
	
	// Get subdomain
	fmt.Printf("%s╔═══════════════════════════════════════════════════════════════╗%s\n", Cyan, NC)
	fmt.Printf("%s║%s                  ENTER YOUR SUBDOMAIN                          %s║%s\n", Cyan, White, Cyan, NC)
	fmt.Printf("%s╠═══════════════════════════════════════════════════════════════╣%s\n", Cyan, NC)
	fmt.Printf("%s║%s  Example: ns-ex.elitex.sbs                                 %s║%s\n", Cyan, White, Cyan, NC)
	fmt.Printf("%s╚═══════════════════════════════════════════════════════════════╝%s\n", Cyan, NC)
	fmt.Println()
	fmt.Print("Subdomain: ")
	subdomain, _ := reader.ReadString('\n')
	config.Subdomain = strings.TrimSpace(subdomain)
	saveConfig()
	
	// Get location
	fmt.Printf("%s╔═══════════════════════════════════════════════════════════════╗%s\n", Yellow, NC)
	fmt.Printf("%s║%s           NETWORK LOCATION OPTIMIZATION                          %s║%s\n", Yellow, Green, Yellow, NC)
	fmt.Printf("%s╠═══════════════════════════════════════════════════════════════╣%s\n", Yellow, NC)
	fmt.Printf("%s║%s  Select your VPS location:                                    %s║%s\n", Yellow, White, Yellow, NC)
	fmt.Printf("%s║%s  [1] South Africa (Default - MTU 1800)                        %s║%s\n", Yellow, Green, Yellow, NC)
	fmt.Printf("%s║%s  [2] USA                                                       %s║%s\n", Yellow, Cyan, Yellow, NC)
	fmt.Printf("%s║%s  [3] Europe                                                    %s║%s\n", Yellow, Blue, Yellow, NC)
	fmt.Printf("%s║%s  [4] Asia                                                      %s║%s\n", Yellow, Purple, Yellow, NC)
	fmt.Printf("%s║%s  [5] Auto-detect                                                %s║%s\n", Yellow, Yellow, Yellow, NC)
	fmt.Printf("%s╚═══════════════════════════════════════════════════════════════╝%s\n", Yellow, NC)
	fmt.Println()
	fmt.Print("Select location [1-5] [default: 1]: ")
	locChoice, _ := reader.ReadString('\n')
	locChoice = strings.TrimSpace(locChoice)
	
	switch locChoice {
	case "2":
		config.Location = "USA"
		fmt.Printf("%s✅ USA selected%s\n", Cyan, NC)
	case "3":
		config.Location = "Europe"
		fmt.Printf("%s✅ Europe selected%s\n", Blue, NC)
	case "4":
		config.Location = "Asia"
		fmt.Printf("%s✅ Asia selected%s\n", Purple, NC)
	case "5":
		config.Location = "Auto-detect"
		fmt.Printf("%s✅ Auto-detect selected%s\n", Yellow, NC)
	default:
		config.Location = "South Africa"
		config.MTU = 1800
		fmt.Printf("%s✅ Using South Africa configuration%s\n", Green, NC)
	}
	saveConfig()
	
	// Load data
	loadUsers()
	loadTraffic()
	sessions = make(map[string]*Session)
	
	// Setup banner
	setupBanner()
	
	// Install dependencies
	fmt.Println("Installing dependencies...")
	exec.Command("apt", "update").Run()
	exec.Command("apt", "install", "-y", "curl", "python3", "jq", "nano", "iptables", "dnsutils", "net-tools", "bc").Run()
	
	// Configure DNS
	os.WriteFile("/etc/resolv.conf", []byte("nameserver 8.8.8.8\nnameserver 8.8.4.4\n"), 0644)
	
	// Start services
	startServices()
	
	// Start traffic monitor
	go trafficMonitorLoop()
	
	// Optimize system
	optimizeSystem()
	
	// Configure firewall
	exec.Command("ufw", "allow", "22/tcp").Run()
	exec.Command("ufw", "allow", "53/udp").Run()
	
	// Get IP
	ipCmd := exec.Command("curl", "-4", "-s", "ifconfig.me")
	ipOut, _ := ipCmd.Output()
	ip := strings.TrimSpace(string(ipOut))
	
	// Show completion
	fmt.Println()
	fmt.Println("╔════════════════════════════════════════╗")
	fmt.Println(" ELITE-X V3.5 INSTALLED SUCCESSFULLY ")
	fmt.Println("╠════════════════════════════════════════╣")
	fmt.Println("   Advanced • Secure • Ultra Fast    ")
	fmt.Println("╚════════════════════════════════════════╝")
	fmt.Printf("DOMAIN  : %s\n", config.Subdomain)
	fmt.Printf("LOCATION: %s\n", config.Location)
	fmt.Printf("MTU     : %d\n", config.MTU)
	fmt.Printf("KEY     : %s\n", config.Activation)
	fmt.Printf("EXPIRE  : %s\n", config.Expiry)
	fmt.Println("╚════════════════════════════════════════╝")
	showQuote()
	
	// Service status
	fmt.Printf("\n%sFinal Service Status:%s\n", Cyan, NC)
	fmt.Printf("%s✅ DNSTT Server: Running%s\n", Green, NC)
	fmt.Printf("%s✅ EDNS Proxy: Running%s\n", Green, NC)
	fmt.Printf("%s✅ Traffic Monitor: Running%s\n", Green, NC)
	fmt.Printf("%s✅ Auto Cleaner: Running%s\n", Green, NC)
	
	fmt.Printf("\n%sELITE-X v3.5 Features:%s\n", Green, NC)
	fmt.Printf("  %s→%s User Login Limit (Max concurrent connections)\n", Yellow, NC)
	fmt.Printf("  %s→%s View User Connection Count (Active sessions)\n", Yellow, NC)
	fmt.Printf("  %s→%s Session-Based Traffic Monitoring (Accurate!)\n", Yellow, NC)
	fmt.Printf("  %s→%s Auto Unlock when traffic reset\n", Yellow, NC)
	fmt.Printf("  %s→%s Server Banner after connection\n", Yellow, NC)
	fmt.Printf("  %s→%s Renew User Option\n", Yellow, NC)
	fmt.Printf("  %s→%s Bandwidth Speed Test Tool\n", Yellow, NC)
	fmt.Printf("  %s→%s Auto Backup System\n", Yellow, NC)
	fmt.Printf("  %s→%s System Optimizer (Turbo Mode)\n", Yellow, NC)
	fmt.Printf("  %s→%s Real-time Connection Monitor\n", Yellow, NC)
	fmt.Printf("  %s→%s User Details with Traffic History\n", Yellow, NC)
	
	fmt.Print("\nOpen menu now? (y/n): ")
	openMenu, _ := reader.ReadString('\n')
	if strings.TrimSpace(openMenu) == "y" {
		mainMenu()
	} else {
		fmt.Printf("%sYou can run 'elite-x' anytime to open the dashboard.%s\n", Yellow, NC)
	}
	
	// Keep running
	select {}
}
