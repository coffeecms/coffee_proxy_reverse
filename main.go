package main

import (
	"fmt"
	"gopkg.in/ini.v1"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
	"golang.org/x/time/rate"
	"path/filepath"
	"io/ioutil"
	"github.com/fsnotify/fsnotify"
)

type Config struct {
	RateLimiting struct {
		RequestsPerSecond int
		BurstLimit        int
	}
	Timeouts struct {
		ReadTimeout  int
		WriteTimeout int
		IdleTimeout  int
	}
	RequestLimits struct {
		MaxRequestSize int64
	}
	SSL struct {
		Enabled  bool
		CertFile string
		KeyFile  string
	}
	Whitelist struct {
		IPs []string
	}
	Blacklist struct {
		IPs []string
	}
}

var (
	config      Config
	proxyMap    = make(map[string]*httputil.ReverseProxy)
	mutex       sync.RWMutex
	workerPool  chan func()
	rateLimiter = make(map[string]*rate.Limiter)
	limiterLock sync.Mutex
)

func loadConfig(filePath string) error {
	cfg, err := ini.Load(filePath)
	if err != nil {
		return err
	}

	// Load rate limiting config
	config.RateLimiting.RequestsPerSecond = cfg.Section("rate_limiting").Key("requests_per_second").MustInt(1)
	config.RateLimiting.BurstLimit = cfg.Section("rate_limiting").Key("burst_limit").MustInt(5)

	// Load timeouts config
	config.Timeouts.ReadTimeout = cfg.Section("timeouts").Key("read_timeout").MustInt(5)
	config.Timeouts.WriteTimeout = cfg.Section("timeouts").Key("write_timeout").MustInt(10)
	config.Timeouts.IdleTimeout = cfg.Section("timeouts").Key("idle_timeout").MustInt(30)

	// Load request limits
	config.RequestLimits.MaxRequestSize = cfg.Section("request_limits").Key("max_request_size").MustInt64(1048576)

	// Load SSL config
	config.SSL.Enabled = cfg.Section("ssl").Key("enabled").MustBool(true)
	config.SSL.CertFile = cfg.Section("ssl").Key("cert_file").String()
	config.SSL.KeyFile = cfg.Section("ssl").Key("key_file").String()

	// Load whitelist and blacklist IPs
	config.Whitelist.IPs = strings.Split(cfg.Section("whitelist").Key("ips").String(), ",")
	config.Blacklist.IPs = strings.Split(cfg.Section("blacklist").Key("ips").String(), ",")

	return nil
}

func initWorkerPool(numWorkers int) {
	workerPool = make(chan func(), numWorkers)
	for i := 0; i < numWorkers; i++ {
		go worker()
	}
}

func worker() {
	for task := range workerPool {
		task()
	}
}

func getRateLimiter(ip string) *rate.Limiter {
	limiterLock.Lock()
	defer limiterLock.Unlock()

	if limiter, exists := rateLimiter[ip]; exists {
		return limiter
	}

	limiter := rate.NewLimiter(rate.Limit(config.RateLimiting.RequestsPerSecond), config.RateLimiting.BurstLimit)
	rateLimiter[ip] = limiter
	return limiter
}

func rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		limiter := getRateLimiter(ip)
		if !limiter.Allow() {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func ipFilterMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Check blacklist
		for _, blockedIP := range config.Blacklist.IPs {
			if blockedIP == ip {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
		}

		// Check whitelist
		allowed := false
		for _, allowedIP := range config.Whitelist.IPs {
			if allowedIP == ip {
				allowed = true
				break
			}
		}

		if !allowed {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func limitRequestSizeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, config.RequestLimits.MaxRequestSize)
		next.ServeHTTP(w, r)
	})
}

// Load domain proxy configuration from .conf files
func loadDomains(directory string) error {
	files, err := ioutil.ReadDir(directory)
	if err != nil {
		return err
	}

	mutex.Lock()
	defer mutex.Unlock()

	for _, file := range files {
		if filepath.Ext(file.Name()) == ".conf" {
			domain := strings.TrimSuffix(file.Name(), filepath.Ext(file.Name()))
			filePath := filepath.Join(directory, file.Name())
			cfg, err := ini.Load(filePath)
			if err != nil {
				log.Printf("Error loading config for domain %s: %v", domain, err)
				continue
			}

			backendURL := cfg.Section("proxy").Key("backend_url").String()
			proxyMap[domain] = newReverseProxy(backendURL)
			fmt.Printf("Loaded proxy for domain: %s -> %s\n", domain, backendURL)
		}
	}
	return nil
}

func newReverseProxy(target string) *httputil.ReverseProxy {
	url, _ := url.Parse(target)
	return &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = url.Scheme
			req.URL.Host = url.Host
		},
	}
}

// Watch for changes in domain config directory
func watchDomains(directory string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Remove == fsnotify.Remove {
					fmt.Println("Domain configuration changed. Reloading...")
					loadDomains(directory)
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("Error watching domain directory:", err)
			}
		}
	}()

	err = watcher.Add(directory)
	if err != nil {
		log.Fatal(err)
	}
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	domain := r.Host
	mutex.RLock()
	proxy, exists := proxyMap[domain]
	mutex.RUnlock()

	if exists {
		workerPool <- func() {
			proxy.ServeHTTP(w, r)
		}
	} else {
		http.Error(w, "Domain not found", http.StatusNotFound)
	}
}

func main() {
	// Load global system config
	err := loadConfig("system.conf")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Load domain proxies
	err = loadDomains("./list_domain")
	if err != nil {
		log.Fatalf("Failed to load domain proxies: %v", err)
	}

	// Watch for changes in domain configurations
	go watchDomains("./list_domain")

	// Initialize worker pool
	initWorkerPool(100)

	// Setup server with timeouts and optional TLS
	server := &http.Server{
		Addr:         ":8080",
		ReadTimeout:  time.Duration(config.Timeouts.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(config.Timeouts.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(config.Timeouts.IdleTimeout) * time.Second,
		Handler:      rateLimitMiddleware(ipFilterMiddleware(limitRequestSizeMiddleware(http.HandlerFunc(proxyHandler)))),
	}

	if config.SSL.Enabled {
		fmt.Println("Starting HTTPS server...")
		log.Fatal(server.ListenAndServeTLS(config.SSL.CertFile, config.SSL.KeyFile))
	} else {
		fmt.Println("Starting HTTP server...")
		log.Fatal(server.ListenAndServe())
	}
}
