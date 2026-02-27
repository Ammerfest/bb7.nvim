package state

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestAcquireReleaseLock(t *testing.T) {
	dir := t.TempDir()
	chatDir := filepath.Join(dir, "test-chat")
	os.MkdirAll(chatDir, 0755)

	// Acquire lock
	if err := AcquireLock(chatDir); err != nil {
		t.Fatalf("AcquireLock failed: %v", err)
	}

	// Lock file should exist with our PID
	data, err := os.ReadFile(lockFilePath(chatDir))
	if err != nil {
		t.Fatalf("Lock file not created: %v", err)
	}
	pid, _ := strconv.Atoi(string(data))
	if pid != os.Getpid() {
		t.Errorf("Lock file PID = %d, want %d", pid, os.Getpid())
	}

	// Release lock
	if err := ReleaseLock(chatDir); err != nil {
		t.Fatalf("ReleaseLock failed: %v", err)
	}

	// Lock file should be gone
	if _, err := os.Stat(lockFilePath(chatDir)); !os.IsNotExist(err) {
		t.Error("Lock file still exists after release")
	}
}

func TestIsLockedOwnProcess(t *testing.T) {
	dir := t.TempDir()
	chatDir := filepath.Join(dir, "test-chat")
	os.MkdirAll(chatDir, 0755)

	// Our own lock should not count as "locked"
	AcquireLock(chatDir)
	defer ReleaseLock(chatDir)

	if IsLocked(chatDir) {
		t.Error("IsLocked should return false for own process lock")
	}
}

func TestStaleLockAutoRemoved(t *testing.T) {
	dir := t.TempDir()
	chatDir := filepath.Join(dir, "test-chat")
	os.MkdirAll(chatDir, 0755)

	// Write a lock file with a dead PID (use a very high PID unlikely to exist)
	deadPID := 999999999
	os.WriteFile(lockFilePath(chatDir), []byte(strconv.Itoa(deadPID)), 0644)

	// IsLocked should detect the stale lock and remove it
	if IsLocked(chatDir) {
		t.Error("IsLocked should return false for dead PID and auto-remove stale lock")
	}

	// Lock file should be removed
	if _, err := os.Stat(lockFilePath(chatDir)); !os.IsNotExist(err) {
		t.Error("Stale lock file not auto-removed")
	}
}

func TestForceUnlock(t *testing.T) {
	dir := t.TempDir()
	chatDir := filepath.Join(dir, "test-chat")
	os.MkdirAll(chatDir, 0755)

	AcquireLock(chatDir)

	if err := ForceUnlock(chatDir); err != nil {
		t.Fatalf("ForceUnlock failed: %v", err)
	}

	if _, err := os.Stat(lockFilePath(chatDir)); !os.IsNotExist(err) {
		t.Error("Lock file still exists after ForceUnlock")
	}
}

func TestForceUnlockNoLock(t *testing.T) {
	dir := t.TempDir()
	chatDir := filepath.Join(dir, "test-chat")
	os.MkdirAll(chatDir, 0755)

	// ForceUnlock on a non-existent lock should not error
	if err := ForceUnlock(chatDir); err != nil {
		t.Fatalf("ForceUnlock on non-existent lock should not error: %v", err)
	}
}

func TestCorruptLockFileAutoRemoved(t *testing.T) {
	dir := t.TempDir()
	chatDir := filepath.Join(dir, "test-chat")
	os.MkdirAll(chatDir, 0755)

	// Write a corrupt lock file
	os.WriteFile(lockFilePath(chatDir), []byte("not-a-pid"), 0644)

	if IsLocked(chatDir) {
		t.Error("IsLocked should return false for corrupt lock file")
	}

	// Corrupt file should be removed
	if _, err := os.Stat(lockFilePath(chatDir)); !os.IsNotExist(err) {
		t.Error("Corrupt lock file not auto-removed")
	}
}

func TestChatSelectAcquiresLock(t *testing.T) {
	s := setupTestState(t)
	chat, err := s.ChatNew("test", "")
	if err != nil {
		t.Fatalf("ChatNew failed: %v", err)
	}

	// Chat should be locked by us
	chatDir := s.chatDir(chat.ID)
	data, err := os.ReadFile(lockFilePath(chatDir))
	if err != nil {
		t.Fatalf("Lock file not created after ChatNew: %v", err)
	}
	pid, _ := strconv.Atoi(string(data))
	if pid != os.Getpid() {
		t.Errorf("Lock PID = %d, want %d", pid, os.Getpid())
	}

	// Create a second chat
	chat2, err := s.ChatNew("test2", "")
	if err != nil {
		t.Fatalf("ChatNew(2) failed: %v", err)
	}

	// First chat's lock should be released
	if _, err := os.Stat(lockFilePath(chatDir)); !os.IsNotExist(err) {
		t.Error("First chat's lock should be released after creating second chat")
	}

	// Second chat should be locked
	chatDir2 := s.chatDir(chat2.ID)
	if _, err := os.Stat(lockFilePath(chatDir2)); os.IsNotExist(err) {
		t.Error("Second chat should be locked")
	}
}

func TestChatListShowsLockStatus(t *testing.T) {
	s := setupTestState(t)

	chat1, _ := s.ChatNew("first", "")
	s.ChatNew("second", "")

	// first chat's lock was released when second was created
	// second chat is locked (by us, which IsLocked ignores)
	chats, err := s.ChatList()
	if err != nil {
		t.Fatalf("ChatList failed: %v", err)
	}

	for _, c := range chats {
		if c.ID == chat1.ID && c.Locked {
			t.Errorf("Chat %s should not be locked (lock was released)", c.ID)
		}
	}
}

func TestCleanupReleasesLock(t *testing.T) {
	s := setupTestState(t)
	chat, _ := s.ChatNew("test", "")
	chatDir := s.chatDir(chat.ID)

	// Lock should exist
	if _, err := os.Stat(lockFilePath(chatDir)); os.IsNotExist(err) {
		t.Fatal("Lock should exist before cleanup")
	}

	s.Cleanup()

	// Lock should be gone
	if _, err := os.Stat(lockFilePath(chatDir)); !os.IsNotExist(err) {
		t.Error("Lock should be released after Cleanup")
	}
}
