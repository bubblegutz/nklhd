package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/bubblegutz/nklhd/internal/config"
	"github.com/bubblegutz/nklhd/internal/fuse"
	"github.com/bubblegutz/nklhd/internal/lua"
	"github.com/bubblegutz/nklhd/internal/ninep"
	"github.com/bubblegutz/nklhd/internal/script"
	"github.com/bubblegutz/nklhd/internal/sshfs"
	"github.com/bubblegutz/nklhd/internal/tengo"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("nklhd: ")

	var (
		cfgFile    string
		mountPoint string
		umount     bool
		verbose    bool
		protocol         string
		listenAddr       string
		sshAddr          string
		sshAuthorizedKeys string
		sshHostKey       string
	)

	flag.StringVar(&cfgFile, "config", "", "config file (name or path)")
	flag.StringVar(&cfgFile, "c", "", "config file (short)")
	flag.StringVar(&mountPoint, "mount", "", "mount point for FUSE")
	flag.StringVar(&mountPoint, "m", "", "mount point for FUSE (short)")
	flag.BoolVar(&umount, "umount", false, "unmount the mount point instead of mounting")
	flag.BoolVar(&umount, "u", false, "unmount (short)")
	flag.BoolVar(&verbose, "verbose", false, "enable verbose debug logging")
	flag.BoolVar(&verbose, "v", false, "verbose (short)")
	flag.StringVar(&protocol, "protocol", "fuse", "filesystem protocol: fuse, 9p, ssh, both, or all")
	flag.StringVar(&listenAddr, "listen", "localhost:5640", "9p listen address")
	flag.StringVar(&sshAddr, "ssh-addr", "localhost:5022", "SSH listen address")
	flag.StringVar(&sshAuthorizedKeys, "ssh-authorized-keys", "", "path to authorized_keys file")
	flag.StringVar(&sshHostKey, "ssh-host-key", "", "path to SSH host key file")
	flag.Parse()

	// Load configuration
	var cfg *config.Config
	var err error
	if cfgFile != "" {
		cfg, err = config.LoadConfigWithPath(cfgFile)
	} else {
		cfg, err = config.LoadConfig()
	}
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Override mount point if specified via flag
	if mountPoint != "" {
		cfg.MountPoint = mountPoint
	}
	// Override verbose if specified via flag
	if verbose {
		cfg.Verbose = true
	}
	// Override protocol if specified via flag
	if protocol != "fuse" {
		cfg.Protocol = protocol
	}
	// Override 9p listen address if specified via flag
	if listenAddr != "localhost:5640" {
		cfg.NinepAddr = listenAddr
	}
	// Override SSH addr if specified via flag
	if sshAddr != "localhost:5022" {
		cfg.SSHAddr = sshAddr
	}
	// Override SSH authorized keys if specified via flag
	if sshAuthorizedKeys != "" {
		cfg.SSHAuthorizedKeys = sshAuthorizedKeys
	}
	// Override SSH host key if specified via flag
	if sshHostKey != "" {
		cfg.SSHHostKey = sshHostKey
	}

	// Default protocol to fuse if not specified
	if cfg.Protocol == "" {
		cfg.Protocol = "fuse"
	}

	// Set debug logging
	if cfg.Verbose {
		log.Println("Verbose mode enabled")
		fuse.SetDebug(true)
		lua.SetDebug(true)
		tengo.SetDebug(true)
	}

	if umount {
		// Unmount mode
		mp := cfg.MountPoint
		if mp == "" {
			log.Fatal("mount point not specified")
		}
		log.Printf("Unmounting %s...", mp)
		if err := fuse.Unmount(mp, nil); err != nil {
			log.Fatalf("failed to unmount: %v", err)
		}
		log.Println("Unmounted successfully")
		return
	}

	// Mount mode
	if cfg.RootScript == "" {
		log.Fatal("root script not specified")
	}
	if cfg.Protocol != "fuse" && cfg.Protocol != "9p" && cfg.Protocol != "ssh" && cfg.Protocol != "both" && cfg.Protocol != "all" {
		log.Fatalf("unsupported protocol: %s (use 'fuse', '9p', 'ssh', 'both', or 'all')", cfg.Protocol)
	}
	if (cfg.Protocol == "fuse" || cfg.Protocol == "both" || cfg.Protocol == "all") && cfg.MountPoint == "" {
		log.Fatal("mount point not specified")
	}
	if (cfg.Protocol == "9p" || cfg.Protocol == "both" || cfg.Protocol == "all") && cfg.NinepAddr == "" {
		cfg.NinepAddr = "localhost:5640"
	}
	if (cfg.Protocol == "ssh" || cfg.Protocol == "all") && cfg.SSHAddr == "" {
		cfg.SSHAddr = "localhost:5022"
	}

	log.Printf("Starting nklhd with root script: %s", cfg.RootScript)

	// Create backend (Lua or Tengo) based on root script extension
	var backend script.Backend
	if strings.HasSuffix(cfg.RootScript, ".tengo") {
		log.Println("Using Tengo scripting engine")
		backend = tengo.NewExecutor()
	} else {
		log.Println("Using Lua scripting engine")
		backend = lua.NewExecutor()
	}
	if err := backend.LoadScript(cfg.RootScript); err != nil {
		backend.Close()
		log.Fatalf("failed to load config: %v", err)
	}
	defer backend.Close()

	// Create servers based on protocol
	var fuseServer *fuse.Server
	var ninepServer *ninep.Server
	var sshfsServer *sshfs.Server

	if cfg.Protocol == "fuse" || cfg.Protocol == "both" || cfg.Protocol == "all" {
		log.Printf("Mount point: %s", cfg.MountPoint)

		// Ensure mount point directory exists
		if err := os.MkdirAll(cfg.MountPoint, 0755); err != nil {
			log.Fatalf("failed to create mount directory: %v", err)
		}

		// Create FUSE server with mount options
		opts := &fuse.MountOptions{
			Debug: cfg.Verbose,
		}
		fuseServer = fuse.NewServer(backend.GetRouter(), opts)
		if err := fuseServer.Mount(cfg.MountPoint); err != nil {
			backend.Close()
			log.Fatalf("FUSE mount failed: %v", err)
		}
	}

	if cfg.Protocol == "9p" || cfg.Protocol == "both" || cfg.Protocol == "all" {
		log.Printf("9p listen address: %s", cfg.NinepAddr)
		ninepServer = ninep.NewServer(backend.GetRouter(), cfg.Verbose)
	}

	if cfg.Protocol == "ssh" || cfg.Protocol == "all" {
		log.Printf("SSH listen address: %s", cfg.SSHAddr)
		sshfsServer = sshfs.NewServer(backend.GetRouter(), cfg.SSHAddr, cfg.SSHAuthorizedKeys, cfg.SSHHostKey, cfg.Verbose)
	}

	// Print status
	switch cfg.Protocol {
	case "all":
		fmt.Printf("nklhd FUSE + 9p + SSH filesystem running\n")
		fmt.Printf("Mount point: %s\n", cfg.MountPoint)
		fmt.Printf("9p address: %s\n", cfg.NinepAddr)
		fmt.Printf("SSH address: %s\n", cfg.SSHAddr)
	case "both":
		fmt.Printf("nklhd FUSE + 9p filesystem running\n")
		fmt.Printf("Mount point: %s\n", cfg.MountPoint)
		fmt.Printf("9p address: %s\n", cfg.NinepAddr)
	case "fuse":
		fmt.Printf("nklhd FUSE filesystem running\n")
		fmt.Printf("Mount point: %s\n", cfg.MountPoint)
	case "9p":
		fmt.Printf("nklhd 9p filesystem running\n")
		fmt.Printf("9p address: %s\n", cfg.NinepAddr)
	case "ssh":
		fmt.Printf("nklhd SSH filesystem running\n")
		fmt.Printf("SSH address: %s\n", cfg.SSHAddr)
	}
	fmt.Printf("Config: %s\n", cfg.RootScript)

	// Start 9p server in background if present
	if ninepServer != nil {
		go func() {
			if err := ninepServer.Serve(cfg.NinepAddr); err != nil {
				log.Printf("9p server stopped: %v", err)
			}
		}()
	}

	// Start SSHFS server in background if present
	if sshfsServer != nil {
		go func() {
			if err := sshfsServer.Serve(); err != nil {
				log.Printf("SSH server stopped: %v", err)
			}
		}()
	}

	// Wait for signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	fmt.Println("\nShutting down...")

	// Graceful shutdown — unmount FUSE first, then stop 9p and SSH
	if fuseServer != nil {
		if err := fuseServer.Unmount(); err != nil {
			log.Printf("warning: unmount failed: %v", err)
		}
	}
	if ninepServer != nil {
		ninepServer.Stop()
	}
	if sshfsServer != nil {
		sshfsServer.Stop()
	}
	if fuseServer != nil {
		if err := fuseServer.Wait(); err != nil {
			log.Printf("warning: wait failed: %v", err)
		}
	}
}