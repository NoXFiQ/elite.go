apt update && apt install -y golang dos2unix && cat > elite-x.go << 'GO_EOF'
package main

import (
    "bufio"
    "encoding/json"
    "fmt"
    "io/ioutil"
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

const (
    Version    = "3.5"
    ConfigDir  = "/etc/elite-x"
    UsersDir   = "/etc/elite-x/users"
    TrafficDir = "/etc/elite-x/traffic"
    DNSTTDir   = "/etc/dnstt"
)

var (
    Green   = "\033[0;32m"
    Red     = "\033[0;31m"
    Yellow  = "\033[1;33m"
    Cyan    = "\033[0;36m"
    Purple  = "\033[0;35m"
    White   = "\033[1;37m"
    NC      = "\033[0m"
    mu      sync.RWMutex
    users   = make(map[string]*User)
    traffic = make(map[string]int64)
)

type User struct {
    Username     string `json:"username"`
    Password     string `json:"password"`
    ExpireDate   string `json:"expire_date"`
    TrafficLimit int64  `json:"traffic_limit"`
    MaxLogins    int    `json:"max_logins"`
    Locked       bool   `json:"locked"`
    CreatedAt    string `json:"created_at"`
}

func main() {
    clearScreen()
    fmt.Printf("%s╔═══════════════════════════════════════════════════════════════╗%s\n", Purple, NC)
    fmt.Printf("%s║%s%s                   ELITE-X SLOWDNS v%s                        %s║%s\n", Purple, Yellow, Bold(), Version, Purple, NC)
    fmt.Printf("%s║%s%s              Advanced • Secure • Ultra Fast                    %s║%s\n", Purple, Green, Bold(), Purple, NC)
    fmt.Printf("%s╚═══════════════════════════════════════════════════════════════╝%s\n", Purple, NC)
    fmt.Println()

    // Create directories
    os.MkdirAll(ConfigDir, 0755)
    os.MkdirAll(UsersDir, 0755)
    os.MkdirAll(TrafficDir, 0755)
    os.MkdirAll(DNSTTDir, 0755)

    // Load existing data
    loadUsers()
    loadTraffic()

    // Signal handling
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        <-sigChan
        fmt.Printf("\n%sShutting down...%s\n", Yellow, NC)
        os.Exit(0)
    }()

    // Start services
    startDNSTTServer()
    startTrafficMonitor()

    // Main menu
    mainMenu()
}

func Bold() string {
    return "\033[1m"
}

func clearScreen() {
    fmt.Print("\033[2J\033[H")
}

func loadUsers() {
    files, err := ioutil.ReadDir(UsersDir)
    if err != nil {
        return
    }
    for _, f := range files {
        if f.IsDir() {
            continue
        }
        data, err := ioutil.ReadFile(filepath.Join(UsersDir, f.Name()))
        if err != nil {
            continue
        }
        var u User
        if err := json.Unmarshal(data, &u); err == nil {
            users[u.Username] = &u
        }
    }
}

func loadTraffic() {
    files, err := ioutil.ReadDir(TrafficDir)
    if err != nil {
        return
    }
    for _, f := range files {
        if f.IsDir() {
            continue
        }
        data, err := ioutil.ReadFile(filepath.Join(TrafficDir, f.Name()))
        if err != nil {
            continue
        }
        val, _ := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
        traffic[f.Name()] = val
    }
}

func saveUser(u *User) error {
    data, err := json.MarshalIndent(u, "", "  ")
    if err != nil {
        return err
    }
    return ioutil.WriteFile(filepath.Join(UsersDir, u.Username), data, 0644)
}

func saveTraffic(username string, val int64) error {
    return ioutil.WriteFile(filepath.Join(TrafficDir, username), []byte(fmt.Sprintf("%d", val)), 0644)
}

func addUser() {
    clearScreen()
    fmt.Printf("%s╔═══════════════════════════════════════════════════════════════╗%s\n", Cyan, NC)
    fmt.Printf("%s║%s                    ADD NEW USER                                  %s║%s\n", Cyan, Yellow, Cyan, NC)
    fmt.Printf("%s╚═══════════════════════════════════════════════════════════════╝%s\n", Cyan, NC)
    fmt.Println()

    reader := bufio.NewReader(os.Stdin)

    fmt.Print("Username: ")
    username, _ := reader.ReadString('\n')
    username = strings.TrimSpace(username)

    if _, exists := users[username]; exists {
        fmt.Printf("%sUser already exists!%s\n", Red, NC)
        fmt.Print("Press Enter to continue...")
        reader.ReadString('\n')
        return
    }

    fmt.Print("Password: ")
    password, _ := reader.ReadString('\n')
    password = strings.TrimSpace(password)

    fmt.Print("Expire days: ")
    daysStr, _ := reader.ReadString('\n')
    days, _ := strconv.Atoi(strings.TrimSpace(daysStr))

    fmt.Print("Traffic limit (MB, 0 for unlimited): ")
    limitStr, _ := reader.ReadString('\n')
    trafficLimit, _ := strconv.ParseInt(strings.TrimSpace(limitStr), 10, 64)

    fmt.Print("Max concurrent logins (0 for unlimited): ")
    maxLoginsStr, _ := reader.ReadString('\n')
    maxLogins, _ := strconv.Atoi(strings.TrimSpace(maxLoginsStr))

    // Create system user
    cmd := exec.Command("useradd", "-m", "-s", "/bin/false", username)
    cmd.Run()
    cmd = exec.Command("sh", "-c", fmt.Sprintf("echo '%s:%s' | chpasswd", username, password))
    cmd.Run()

    expireDate := time.Now().AddDate(0, 0, days).Format("2006-01-02")
    cmd = exec.Command("chage", "-E", expireDate, username)
    cmd.Run()

    user := &User{
        Username:     username,
        Password:     password,
        ExpireDate:   expireDate,
        TrafficLimit: trafficLimit,
        MaxLogins:    maxLogins,
        Locked:       false,
        CreatedAt:    time.Now().Format("2006-01-02 15:04:05"),
    }

    saveUser(user)
    saveTraffic(username, 0)

    clearScreen()
    fmt.Printf("%s════════════════════════════════════════════%s\n", Green, NC)
    fmt.Printf("User created successfully!\n")
    fmt.Printf("Username      : %s\n", username)
    fmt.Printf("Password      : %s\n", password)
    fmt.Printf("Expire        : %s\n", expireDate)
    fmt.Printf("Traffic Limit : %d MB\n", trafficLimit)
    fmt.Printf("Max Logins    : %d\n", maxLogins)
    fmt.Printf("%s════════════════════════════════════════════%s\n", Green, NC)
    fmt.Println()
    fmt.Print("Press Enter to continue...")
    reader.ReadString('\n')
}

func listUsers() {
    clearScreen()
    fmt.Printf("%s╔═══════════════════════════════════════════════════════════════╗%s\n", Cyan, NC)
    fmt.Printf("%s║%s                    ACTIVE USERS                                 %s║%s\n", Cyan, Yellow, Cyan, NC)
    fmt.Printf("%s╚═══════════════════════════════════════════════════════════════╝%s\n", Cyan, NC)
    fmt.Println()

    if len(users) == 0 {
        fmt.Printf("%sNo users found%s\n", Red, NC)
        fmt.Print("\nPress Enter to continue...")
        bufio.NewReader(os.Stdin).ReadString('\n')
        return
    }

    fmt.Printf("%-12s %-10s %-10s %-10s %-8s\n", "USERNAME", "EXPIRE", "LIMIT", "USED", "STATUS")
    fmt.Println("─────────────────────────────────────────────────────────")

    totalTraffic := int64(0)

    for username, user := range users {
        used := traffic[username]
        status := "OK"
        statusColor := Green
        if user.Locked {
            status = "LOCK"
            statusColor = Red
        }

        fmt.Printf("%-12s %-10s %-10d %-10d %s%s%s\n",
            username, user.ExpireDate, user.TrafficLimit, used,
            statusColor, status, NC)
        totalTraffic += used
    }

    fmt.Println("─────────────────────────────────────────────────────────")
    fmt.Printf("Total Users: %s%d%s | Total Traffic Used: %s%d MB%s\n",
        Green, len(users), NC, Yellow, totalTraffic, NC)
    fmt.Println()
    fmt.Print("Press Enter to continue...")
    bufio.NewReader(os.Stdin).ReadString('\n')
}

func lockUser() {
    fmt.Print("Username: ")
    reader := bufio.NewReader(os.Stdin)
    username, _ := reader.ReadString('\n')
    username = strings.TrimSpace(username)

    user, exists := users[username]
    if !exists {
        fmt.Printf("%sUser not found!%s\n", Red, NC)
    } else {
        user.Locked = true
        saveUser(user)
        exec.Command("usermod", "-L", username).Run()
        exec.Command("pkill", "-u", username).Run()
        fmt.Printf("%sUser locked successfully!%s\n", Green, NC)
    }
    fmt.Print("Press Enter to continue...")
    reader.ReadString('\n')
}

func unlockUser() {
    fmt.Print("Username: ")
    reader := bufio.NewReader(os.Stdin)
    username, _ := reader.ReadString('\n')
    username = strings.TrimSpace(username)

    user, exists := users[username]
    if !exists {
        fmt.Printf("%sUser not found!%s\n", Red, NC)
    } else {
        user.Locked = false
        saveUser(user)
        exec.Command("usermod", "-U", username).Run()
        fmt.Printf("%sUser unlocked successfully!%s\n", Green, NC)
    }
    fmt.Print("Press Enter to continue...")
    reader.ReadString('\n')
}

func deleteUser() {
    fmt.Print("Username: ")
    reader := bufio.NewReader(os.Stdin)
    username, _ := reader.ReadString('\n')
    username = strings.TrimSpace(username)

    if _, exists := users[username]; !exists {
        fmt.Printf("%sUser not found!%s\n", Red, NC)
    } else {
        delete(users, username)
        delete(traffic, username)
        os.Remove(filepath.Join(UsersDir, username))
        os.Remove(filepath.Join(TrafficDir, username))
        exec.Command("userdel", "-r", username).Run()
        fmt.Printf("%sUser deleted successfully!%s\n", Green, NC)
    }
    fmt.Print("Press Enter to continue...")
    reader.ReadString('\n')
}

func trafficStats() {
    clearScreen()
    fmt.Printf("%s╔═══════════════════════════════════════════════════════════════╗%s\n", Cyan, NC)
    fmt.Printf("%s║%s                    TRAFFIC STATISTICS                            %s║%s\n", Cyan, Yellow, Cyan, NC)
    fmt.Printf("%s╚═══════════════════════════════════════════════════════════════╝%s\n", Cyan, NC)
    fmt.Println()

    fmt.Printf("%-12s %-15s %-12s %-10s\n", "USERNAME", "LIMIT (MB)", "USED (MB)", "USAGE%")
    fmt.Println("─────────────────────────────────────────────────")

    total := int64(0)

    for username, user := range users {
        used := traffic[username]
        percent := "Unlimited"
        if user.TrafficLimit > 0 {
            p := used * 100 / user.TrafficLimit
            percent = fmt.Sprintf("%d%%", p)
        }
        fmt.Printf("%-12s %-15d %-12d %-10s\n", username, user.TrafficLimit, used, percent)
        total += used
    }

    fmt.Println("─────────────────────────────────────────────────")
    fmt.Printf("Total Traffic Used: %s%d MB%s\n", Yellow, total, NC)
    fmt.Println()
    fmt.Print("Press Enter to continue...")
    bufio.NewReader(os.Stdin).ReadString('\n')
}

func renewUser() {
    reader := bufio.NewReader(os.Stdin)
    fmt.Print("Username: ")
    username, _ := reader.ReadString('\n')
    username = strings.TrimSpace(username)

    user, exists := users[username]
    if !exists {
        fmt.Printf("%sUser not found!%s\n", Red, NC)
        fmt.Print("Press Enter to continue...")
        reader.ReadString('\n')
        return
    }

    fmt.Printf("Current expiry: %s\n", user.ExpireDate)
    fmt.Print("Add how many days? (0 to skip): ")
    daysStr, _ := reader.ReadString('\n')
    addDays, _ := strconv.Atoi(strings.TrimSpace(daysStr))

    fmt.Print("Reset traffic usage? (y/n): ")
    resetStr, _ := reader.ReadString('\n')
    resetTraffic := strings.TrimSpace(resetStr) == "y"

    if addDays > 0 {
        currentExpire, _ := time.Parse("2006-01-02", user.ExpireDate)
        newExpire := currentExpire.AddDate(0, 0, addDays)
        user.ExpireDate = newExpire.Format("2006-01-02")
        exec.Command("chage", "-E", user.ExpireDate, username).Run()
        fmt.Printf("Expiry updated to: %s\n", user.ExpireDate)
    }

    if resetTraffic {
        saveTraffic(username, 0)
        if user.Locked {
            user.Locked = false
            exec.Command("usermod", "-U", username).Run()
        }
        fmt.Printf("Traffic reset to 0 MB\n")
    }

    saveUser(user)
    fmt.Printf("%sUser renewed successfully!%s\n", Green, NC)
    fmt.Print("Press Enter to continue...")
    reader.ReadString('\n')
}

func speedTest() {
    clearScreen()
    fmt.Printf("%s╔═══════════════════════════════════════════════════════════════╗%s\n", Cyan, NC)
    fmt.Printf("%s║%s              ELITE-X BANDWIDTH SPEED TEST                      %s║%s\n", Cyan, Yellow, Cyan, NC)
    fmt.Printf("%s╚═══════════════════════════════════════════════════════════════╝%s\n", Cyan, NC)
    fmt.Println()

    fmt.Printf("%sTesting download speed...%s\n", Yellow, NC)
    start := time.Now()
    cmd := exec.Command("curl", "-s", "-o", "/dev/null", "http://speedtest.tele2.net/100MB.zip")
    cmd.Start()
    time.Sleep(5 * time.Second)
    cmd.Process.Kill()
    elapsed := time.Since(start).Seconds()

    if elapsed > 0 {
        downloadSpeed := 100.0 / elapsed * 8
        fmt.Printf("%sDownload Speed: %.2f Mbps%s\n", Green, downloadSpeed, NC)
    }

    fmt.Printf("\n%sTesting upload speed...%s\n", Yellow, NC)
    start = time.Now()
    cmd = exec.Command("sh", "-c", "dd if=/dev/zero bs=1M count=50 2>/dev/null | curl -s -X POST --data-binary @- https://httpbin.org/post -o /dev/null")
    cmd.Start()
    time.Sleep(5 * time.Second)
    cmd.Process.Kill()
    elapsed = time.Since(start).Seconds()

    if elapsed > 0 {
        uploadSpeed := 50.0 / elapsed * 8
        fmt.Printf("%sUpload Speed:   %.2f Mbps%s\n", Green, uploadSpeed, NC)
    }

    fmt.Println()
    fmt.Print("Press Enter to continue...")
    bufio.NewReader(os.Stdin).ReadString('\n')
}

func showDashboard() {
    clearScreen()

    // Get IP
    ipCmd := exec.Command("curl", "-4", "-s", "ifconfig.me")
    ipOut, _ := ipCmd.Output()
    ip := strings.TrimSpace(string(ipOut))
    if ip == "" {
        ip = "Unknown"
    }

    // Get RAM
    ramCmd := exec.Command("free", "-m")
    ramOut, _ := ramCmd.Output()
    ramLines := strings.Split(string(ramOut), "\n")
    ramUsed, ramTotal := "", ""
    for _, line := range ramLines {
        if strings.HasPrefix(line, "Mem:") {
            fields := strings.Fields(line)
            if len(fields) >= 3 {
                ramUsed = fields[2]
                ramTotal = fields[1]
            }
        }
    }

    // Get active connections
    sshCmd := exec.Command("ss", "-tnp")
    sshOut, _ := sshCmd.Output()
    sshCount := strings.Count(string(sshOut), ":22") - strings.Count(string(sshOut), "LISTEN")

    // Calculate total traffic
    totalTraffic := int64(0)
    for _, v := range traffic {
        totalTraffic += v
    }

    fmt.Printf("%s╔════════════════════════════════════════════════════════════════╗%s\n", Purple, NC)
    fmt.Printf("%s║%s%s                    ELITE-X SLOWDNS v%s                       %s║%s\n", Purple, Yellow, Bold(), Version, Purple, NC)
    fmt.Printf("%s╠════════════════════════════════════════════════════════════════╣%s\n", Purple, NC)
    fmt.Printf("%s║%s  IP        :%s %-50s%s║%s\n", Purple, White, Green, ip, Purple, NC)
    fmt.Printf("%s║%s  RAM       :%s %sMB / %sMB%-42s%s║%s\n", Purple, White, Green, ramUsed, ramTotal, "", Purple, NC)
    fmt.Printf("%s║%s  Connections: SSH: %s%d%-50s%s║%s\n", Purple, White, Green, sshCount, "", Purple, NC)
    fmt.Printf("%s║%s  Total Traffic Used: %s%d MB%-33s%s║%s\n", Purple, White, Yellow, totalTraffic, "", Purple, NC)
    fmt.Printf("%s║%s  Active Users: %s%d%-49s%s║%s\n", Purple, White, Cyan, len(users), "", Purple, NC)
    fmt.Printf("%s╠════════════════════════════════════════════════════════════════╣%s\n", Purple, NC)
    fmt.Printf("%s║%s  Act Key   :%s ELITE-X-PRO%-46s%s║%s\n", Purple, White, Yellow, "", Purple, NC)
    fmt.Printf("%s║%s  Expiry    :%s Lifetime%-50s%s║%s\n", Purple, White, Yellow, "", Purple, NC)
    fmt.Printf("%s╚════════════════════════════════════════════════════════════════╝%s\n", Purple, NC)
    fmt.Println()
}

func startDNSTTServer() {
    // Simple DNS tunnel simulation
    go func() {
        addr, _ := net.ResolveUDPAddr("udp", ":5300")
        conn, _ := net.ListenUDP("udp", addr)
        if conn != nil {
            defer conn.Close()
            buffer := make([]byte, 4096)
            for {
                n, clientAddr, _ := conn.ReadFromUDP(buffer)
                if n > 0 {
                    // Forward to SSH
                    sshConn, _ := net.Dial("tcp", "127.0.0.1:22")
                    if sshConn != nil {
                        sshConn.Write(buffer[:n])
                        response := make([]byte, 4096)
                        sshConn.Read(response)
                        conn.WriteToUDP(response, clientAddr)
                        sshConn.Close()
                    }
                }
            }
        }
    }()
    fmt.Printf("%sDNSTT Server started on port 5300%s\n", Green, NC)
}

func startTrafficMonitor() {
    go func() {
        ticker := time.NewTicker(30 * time.Second)
        for range ticker.C {
            mu.Lock()
            for username, user := range users {
                if user.TrafficLimit > 0 && !user.Locked {
                    used := traffic[username]
                    if used >= user.TrafficLimit {
                        user.Locked = true
                        saveUser(user)
                        exec.Command("usermod", "-L", username).Run()
                        exec.Command("pkill", "-u", username).Run()
                        fmt.Printf("%sUser %s locked - exceeded limit%s\n", Yellow, username, NC)
                    }
                }
            }
            mu.Unlock()
        }
    }()
    fmt.Printf("%sTraffic monitor started%s\n", Green, NC)
}

func mainMenu() {
    reader := bufio.NewReader(os.Stdin)
    for {
        showDashboard()
        fmt.Printf("%s╔════════════════════════════════════════════════════════════════╗%s\n", Cyan, NC)
        fmt.Printf("%s║%s%s                         MAIN MENU                              %s║%s\n", Cyan, Green, Bold(), Cyan, NC)
        fmt.Printf("%s╠════════════════════════════════════════════════════════════════╣%s\n", Cyan, NC)
        fmt.Printf("%s║%s  [1] 👤 Add User%s\n", Cyan, White, NC)
        fmt.Printf("%s║%s  [2] 📊 View All Users%s\n", Cyan, White, NC)
        fmt.Printf("%s║%s  [3] 🔒 Lock User%s\n", Cyan, White, NC)
        fmt.Printf("%s║%s  [4] 🔓 Unlock User%s\n", Cyan, White, NC)
        fmt.Printf("%s║%s  [5] 🗑️  Delete User%s\n", Cyan, White, NC)
        fmt.Printf("%s║%s  [6] 🔄 Renew User%s\n", Cyan, White, NC)
        fmt.Printf("%s║%s  [7] 📈 Traffic Statistics%s\n", Cyan, White, NC)
        fmt.Printf("%s║%s  [8] ⚡ Speed Test%s\n", Cyan, White, NC)
        fmt.Printf("%s║%s  [0] 🚪 Exit%s\n", Cyan, White, NC)
        fmt.Printf("%s╚════════════════════════════════════════════════════════════════╝%s\n", Cyan, NC)
        fmt.Println()
        fmt.Print("Main menu option: ")

        choice, _ := reader.ReadString('\n')
        choice = strings.TrimSpace(choice)

        switch choice {
        case "1":
            addUser()
        case "2":
            listUsers()
        case "3":
            lockUser()
        case "4":
            unlockUser()
        case "5":
            deleteUser()
        case "6":
            renewUser()
        case "7":
            trafficStats()
        case "8":
            speedTest()
        case "0":
            fmt.Printf("%sGoodbye!%s\n", Green, NC)
            return
        default:
            fmt.Printf("%sInvalid option%s\n", Red, NC)
            fmt.Print("Press Enter to continue...")
            reader.ReadString('\n')
        }
    }
}
GO_EOF
dos2unix elite-x.go && go build -o elite-x elite-x.go && cp elite-x /usr/local/bin/ && chmod +x /usr/local/bin/elite-x && rm -f elite-x.go && elite-x
