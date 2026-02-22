package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// lockFilePath returns the path to the lock file for a chat directory.
func lockFilePath(chatDir string) string {
	return filepath.Join(chatDir, "lock")
}

// AcquireLock creates a lock file containing the current PID.
// Returns ErrChatLocked if the chat is already locked by a live process.
func AcquireLock(chatDir string) error {
	lockPath := lockFilePath(chatDir)

	// Check for existing lock
	if isLockedByOther(lockPath) {
		return ErrChatLocked
	}

	// Write current PID
	pid := os.Getpid()
	return os.WriteFile(lockPath, []byte(strconv.Itoa(pid)), 0644)
}

// ReleaseLock removes the lock file. Best-effort: ignores ENOENT.
func ReleaseLock(chatDir string) error {
	err := os.Remove(lockFilePath(chatDir))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// IsLocked checks if a chat directory is locked by another process.
// Auto-removes stale locks (dead PID).
func IsLocked(chatDir string) bool {
	return isLockedByOther(lockFilePath(chatDir))
}

// ForceUnlock removes the lock file unconditionally.
func ForceUnlock(chatDir string) error {
	return ReleaseLock(chatDir)
}

// isLockedByOther checks the lock file and returns true if locked by a live
// process other than the current one. Removes stale locks.
func isLockedByOther(lockPath string) bool {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return false // No lock file or can't read it
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		// Corrupt lock file — remove it
		os.Remove(lockPath)
		return false
	}

	// Our own lock
	if pid == os.Getpid() {
		return false
	}

	// Check if process is alive
	if !isProcessAlive(pid) {
		// Stale lock — remove it
		os.Remove(lockPath)
		return false
	}

	return true
}

// isProcessAlive checks if a process with the given PID exists.
func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 checks existence without actually sending a signal
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

// lockOwnerPID returns the PID from a lock file, or 0 if not locked.
func lockOwnerPID(chatDir string) int {
	data, err := os.ReadFile(lockFilePath(chatDir))
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return pid
}

// lockInfo returns a human-readable description of the lock state.
func lockInfo(chatDir string) string {
	pid := lockOwnerPID(chatDir)
	if pid == 0 {
		return ""
	}
	return fmt.Sprintf("locked by PID %d", pid)
}
