package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

func main() {
	// Define the flag for watch mode
	watch := flag.Bool("watch", true, "Enable continuous sync (default: true)")
	changes := flag.String("changes", "write,remove,chmod,rename", "Changes to watch for - chmod, write, remove (default: write, remove)")
	compressVar := flag.Bool("compress", true, "Enable compression (default: true)")
	verboseVar := flag.Bool("verbose", true, "Enable verbose output (default: true)")

	compress := true
	if compressVar != nil {
		compress = *compressVar
	}

	verbose := true
	if verboseVar != nil {
		verbose = *verboseVar
	}

	flag.Parse()

	if len(flag.Args()) != 1 {
		fmt.Println(`ssync by Alex Ellis, Copyright 2025

Usage: ssync [-watch=false] [-changes "write,delete"] <remote-host>

To ignore large files i.e. binaries, create a .ssyncignore file

Learn more https://github.com/alexellis/ssync
`)
		os.Exit(1)
	}

	remoteHost := flag.Args()[0]

	// Get the current working directory
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error: Unable to get current working directory: %v\n", err)
		os.Exit(1)
	}

	// Get the user's home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Error: Unable to get user's home directory: %v\n", err)
		os.Exit(1)
	}

	// Compute relative path to the home directory
	relativePath, err := filepath.Rel(homeDir, cwd)
	if err != nil {
		fmt.Printf("Error: Unable to compute relative path: %v\n", err)
		os.Exit(1)
	}

	// Define the target path on the remote machine
	remotePath := fmt.Sprintf("%s:%s/%s", remoteHost, "~", relativePath)

	// Load exclusions from the .ssyncignore file
	exclusions, err := loadIgnoreFile(cwd)
	if err != nil {
		fmt.Printf("Error: Unable to load .ssyncignore file: %v\n", err)
		os.Exit(1)
	}

	// Perform an initial sync
	fmt.Printf("ssync - Copyright Alex Ellis 2024\n\n%s\n=>\n%s\n\n", cwd, remotePath)

	runRsync(cwd, remotePath, exclusions, compress, verbose)

	// Check if we should watch for changes
	if *watch {
		fmt.Printf("\nWatching %s for changes...\n", cwd)

		changeList := strings.Split(*changes, ",")
		for i := 0; i < len(changeList); i++ {
			changeList[i] = strings.ToUpper(strings.TrimSpace(changeList[i]))
		}

		startWatcher(cwd, remotePath, exclusions, changeList, compress, verbose)
	} else {
		fmt.Println("Sync completed. Watch mode disabled.")
	}
}

func loadIgnoreFile(dir string) ([]string, error) {
	var exclusions []string
	ignoreFilePath := filepath.Join(dir, ".ssyncignore")

	file, err := os.Open(ignoreFilePath)
	if err != nil {
		// If the file doesn't exist, just return an empty list of exclusions
		if os.IsNotExist(err) {
			return exclusions, nil
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Ignore empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Add the pattern directly (rsync interprets it correctly)
		exclusions = append(exclusions, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return exclusions, nil
}
func runRsync(source, destination string, exclusions []string, compress, verbose bool) {
	rsyncArgs := []string{
		"-a", // Archive mode (recursive), verbose, compress
	}

	if verbose {
		rsyncArgs[0] += "v"
	}

	if compress {
		rsyncArgs[0] += "z"
	}

	// Add exclusions to the rsync arguments
	for _, exclude := range exclusions {
		rsyncArgs = append(rsyncArgs, "--exclude", exclude)
	}

	// Add source and destination
	rsyncArgs = append(rsyncArgs, source+"/", destination)

	cmd := exec.Command("rsync", rsyncArgs...)

	// Pipe stdout and stderr to the console
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the rsync command
	if err := cmd.Run(); err != nil {
		fmt.Printf("Error: rsync command failed: %v\n", err)
	} else {
		fmt.Println("Sync completed successfully.")
	}
}
func startWatcher(source, destination string, exclusions, changeList []string, compress, verbose bool) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Printf("Error: Unable to create file watcher: %v\n", err)
		os.Exit(1)
	}
	defer watcher.Close()

	// Watch source directory
	if err := watcher.Add(source); err != nil {
		fmt.Printf("Error: Unable to watch directory: %v\n", err)
		os.Exit(1)
	}

	// Timer to debounce events
	var syncTimer *time.Timer
	const debounceDelay = 2 * time.Second

	go func() {
		for {
			select {
			case event := <-watcher.Events:

				// Check if the event type matches
				if !isWatchedEvent(event, changeList) {
					continue
				}

				// Check if the file path is excluded
				if isExcluded(event.Name, exclusions) {
					continue
				}

				name := strings.TrimPrefix(event.Name, source)

				name = strings.TrimPrefix(name, "/")

				fmt.Printf("[%s] %s\n", strings.ToLower(event.Op.String()), name)

				// Handle debounce and trigger sync
				if syncTimer != nil {
					syncTimer.Stop()
				}

				syncTimer = time.AfterFunc(debounceDelay, func() {
					runRsync(source, destination, exclusions, compress, verbose)
				})

			case err := <-watcher.Errors:
				fmt.Printf("Error watching files: %v\n", err)
			}
		}
	}()

	// Keep the program running
	select {}
}
func isWatchedEvent(event fsnotify.Event, changeList []string) bool {

	for _, changeType := range changeList {
		switch strings.ToLower(changeType) {
		case "write":
			if event.Op == fsnotify.Write {
				return true
			}
		case "remove":
			if event.Op == fsnotify.Remove {
				return true
			}
		case "chmod":
			if event.Op == fsnotify.Chmod {
				return true
			}
		case "create":
			if event.Op == fsnotify.Create {
				return true
			}
		case "rename":
			if event.Op == fsnotify.Rename {
				return true
			}
		}
	}
	return false
}
func isExcluded(path string, exclusions []string) bool {
	// Normalize the absolute path from fsnotify to a relative path
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error: Unable to get current working directory: %v\n", err)
		return false
	}

	relPath, err := filepath.Rel(cwd, path)
	if err != nil {
		fmt.Printf("Error: Unable to make path relative: %v\n", err)
		return false
	}

	log.Printf("relPath: %s, cwd: %s", relPath, cwd)

	// Match the normalized path against exclusions
	for _, pattern := range exclusions {
		// Debug log for pattern matching

		// Handle wildcard patterns like "*.swp"
		if strings.Contains(pattern, "*") {
			matched, err := filepath.Match(pattern, filepath.Base(relPath))
			if err != nil {
				fmt.Printf("Error: Invalid pattern %s\n", pattern)
				continue
			}
			if matched {
				return true
			}
		}

		// Handle rooted patterns like "/secret"
		if strings.HasPrefix(pattern, "/") {
			trimmed := strings.TrimPrefix(pattern, "/")
			if relPath == trimmed {
				return true
			}
		}

		// Handle general filename matches like "secret"
		if filepath.Base(relPath) == pattern {
			return true
		}
	}

	// If no match, it's not excluded
	return false
}
