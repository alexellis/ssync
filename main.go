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

type syncEndpoint struct {
	name      string
	rsyncPath string
	isLocal   bool
	localPath string
}

func main() {
	// Define the flag for watch mode
	watch := flag.Bool("watch", true, "Enable continuous sync (default: true)")
	changes := flag.String("changes", "write,remove,chmod,rename", "Changes to watch for - chmod, write, remove (default: write, remove)")
	compressVar := flag.Bool("compress", true, "Enable compression (default: true)")
	deleteVar := flag.Bool("delete", false, "Mirror destination by deleting extraneous files (default: false)")
	progressVar := flag.Bool("progress", true, "Enable progress output (default: true)")
	verboseVar := flag.Bool("verbose", true, "Enable verbose output (default: true)")

	compress := true
	if compressVar != nil {
		compress = *compressVar
	}

	progress := true
	if progressVar != nil {
		progress = *progressVar
	}

	delete := false
	if deleteVar != nil {
		delete = *deleteVar
	}

	verbose := true
	if verboseVar != nil {
		verbose = *verboseVar
	}

	flag.Parse()

	args := flag.Args()
	if len(args) == 0 || len(args) > 2 {
		fmt.Print(`ssync by Alex Ellis, Copyright 2025

Usage: ssync [-watch=false] [-changes "write,delete"] [-compress=false] [-progress=false] [-delete]
       ssync <destination>
       ssync . <destination>
       ssync <destination> .

Use "." to represent the current directory. Example flows:
  ssync bq         # same as "ssync . bq" (push local -> remote)
  ssync . bq       # explicit push local -> remote
  ssync bq .       # pull remote -> local

Use "--delete" to mirror the destination (removes files missing from the source)

To ignore large files i.e. binaries, create a .ssyncignore file

[Push mode] The remote folder is created automatically if it doesn't exist
already

[Pull mode] You need to create the folder locally, and cd into it
before running ssync.

Learn more https://github.com/alexellis/ssync
`)
		os.Exit(1)
	}

	sourceArg := "."
	destArg := args[0]

	if len(args) == 2 {
		sourceArg = args[0]
		destArg = args[1]
	}

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

	sourceEndpoint, err := newEndpoint(sourceArg, cwd, relativePath)
	if err != nil {
		fmt.Printf("Error: Unable to determine source: %v\n", err)
		os.Exit(1)
	}

	destEndpoint, err := newEndpoint(destArg, cwd, relativePath)
	if err != nil {
		fmt.Printf("Error: Unable to determine destination: %v\n", err)
		os.Exit(1)
	}

	if !sourceEndpoint.isLocal && !destEndpoint.isLocal {
		fmt.Println("Error: Either the source or destination must be the local machine.")
		os.Exit(1)
	}

	// Load exclusions from the .ssyncignore file
	ignoreBase := cwd
	if sourceEndpoint.isLocal {
		ignoreBase = sourceEndpoint.localPath
	} else if destEndpoint.isLocal {
		ignoreBase = destEndpoint.localPath
	}

	exclusions, err := loadIgnoreFile(ignoreBase)
	if err != nil {
		fmt.Printf("Error: Unable to load .ssyncignore file: %v\n", err)
		os.Exit(1)
	}

	// Perform an initial sync
	fmt.Printf("ssync - Copyright Alex Ellis 2024\n\n%s\n=>\n%s\n\n", sourceEndpoint.name, destEndpoint.name)

	runRsync(sourceEndpoint.rsyncPath, destEndpoint.rsyncPath, exclusions, compress, verbose, progress, delete)

	// Check if we should watch for changes
	if *watch {
		if sourceEndpoint.isLocal {
			fmt.Printf("\nWatching %s for changes...\n", sourceEndpoint.localPath)

			changeList := strings.Split(*changes, ",")
			for i := 0; i < len(changeList); i++ {
				changeList[i] = strings.ToUpper(strings.TrimSpace(changeList[i]))
			}

			startWatcher(sourceEndpoint.localPath, destEndpoint.rsyncPath, exclusions, changeList, compress, verbose, progress, delete)
		} else {
			fmt.Println("Watch mode is only available when syncing from the local machine. Skipping watcher.")
		}
	} else {
		fmt.Println("Sync completed. Watch mode disabled.")
	}
}

func newEndpoint(arg, cwd, relativePath string) (syncEndpoint, error) {
	cleanCwd := filepath.Clean(cwd)

	if arg == "" || arg == "." || strings.HasPrefix(arg, "./") || strings.HasPrefix(arg, "../") || filepath.IsAbs(arg) {
		localPath := cleanCwd
		if arg != "" && arg != "." {
			absPath, err := filepath.Abs(arg)
			if err != nil {
				return syncEndpoint{}, err
			}
			localPath = filepath.Clean(absPath)
		}

		return syncEndpoint{
			name:      localPath,
			rsyncPath: localPath,
			isLocal:   true,
			localPath: localPath,
		}, nil
	}

	remotePath := formatRemotePath(arg, relativePath)

	return syncEndpoint{
		name:      remotePath,
		rsyncPath: remotePath,
		isLocal:   false,
	}, nil
}

func formatRemotePath(host, relativePath string) string {
	trimmed := relativePath
	if trimmed == "" {
		trimmed = "."
	}

	trimmed = strings.TrimPrefix(trimmed, "./")
	trimmed = strings.TrimPrefix(trimmed, "/")

	remoteBase := "~"
	if trimmed == "." || trimmed == "" {
		return fmt.Sprintf("%s:%s/", host, remoteBase)
	}

	if trimmed != "" {
		remoteBase = fmt.Sprintf("%s/%s", remoteBase, trimmed)
	}

	return fmt.Sprintf("%s:%s", host, remoteBase)
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
func runRsync(source, destination string, exclusions []string, compress, verbose, progress, delete bool) {
	rsyncArgs := []string{
		"-a", // Archive mode (recursive), verbose, compress
	}

	if verbose {
		rsyncArgs[0] += "v"
	}

	if compress {
		rsyncArgs[0] += "z"
	}

	if progress {
		rsyncArgs = append(rsyncArgs, "--progress")
	}

	if delete {
		rsyncArgs = append(rsyncArgs, "--delete")
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
func startWatcher(source, destination string, exclusions, changeList []string, compress, verbose, progress, delete bool) {
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
					runRsync(source, destination, exclusions, compress, verbose, progress, delete)
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
