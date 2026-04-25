package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/viper"
	"github.com/swim233/logger"
)

// permissions stores "chatID:userID" -> ["summary", "switch", ...]
var (
	permMu   sync.RWMutex
	permData map[string][]string
)

func permissionsPath() string {
	dir := filepath.Dir(viper.ConfigFileUsed())
	return filepath.Join(dir, "permissions.json")
}

func LoadPermissions() {
	permMu.Lock()
	defer permMu.Unlock()

	permData = make(map[string][]string)
	path := permissionsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Info("permissions.json not found, starting with empty permissions")
			return
		}
		logger.Error("failed to read permissions.json: %s", err.Error())
		return
	}
	if err := json.Unmarshal(data, &permData); err != nil {
		logger.Error("failed to parse permissions.json: %s", err.Error())
		permData = make(map[string][]string)
	}
}

func savePermissions() error {
	data, err := json.MarshalIndent(permData, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(permissionsPath(), data, 0644)
}

func permKey(chatID, userID int64) string {
	return fmt.Sprintf("%d:%d", chatID, userID)
}

func GrantPermission(chatID, userID int64, command string) error {
	permMu.Lock()
	defer permMu.Unlock()

	key := permKey(chatID, userID)
	for _, c := range permData[key] {
		if c == command {
			return nil // already granted
		}
	}
	permData[key] = append(permData[key], command)
	return savePermissions()
}

func RevokePermission(chatID, userID int64, command string) error {
	permMu.Lock()
	defer permMu.Unlock()

	key := permKey(chatID, userID)
	cmds := permData[key]
	for i, c := range cmds {
		if c == command {
			permData[key] = append(cmds[:i], cmds[i+1:]...)
			if len(permData[key]) == 0 {
				delete(permData, key)
			}
			return savePermissions()
		}
	}
	return nil
}

func RevokeAllPermissions(chatID, userID int64) error {
	permMu.Lock()
	defer permMu.Unlock()

	key := permKey(chatID, userID)
	if _, ok := permData[key]; !ok {
		return nil
	}
	delete(permData, key)
	return savePermissions()
}

func HasPermission(chatID, userID int64, command string) bool {
	permMu.RLock()
	defer permMu.RUnlock()

	key := permKey(chatID, userID)
	for _, c := range permData[key] {
		if c == command {
			return true
		}
	}
	return false
}

func GetPermissions(chatID, userID int64) []string {
	permMu.RLock()
	defer permMu.RUnlock()

	return append([]string(nil), permData[permKey(chatID, userID)]...)
}

// ListChatPermissions 返回指定群组的所有授权信息
func ListChatPermissions(chatID int64) map[int64][]string {
	permMu.RLock()
	defer permMu.RUnlock()

	prefix := fmt.Sprintf("%d:", chatID)
	result := make(map[int64][]string)
	for key, cmds := range permData {
		if strings.HasPrefix(key, prefix) {
			uidStr := strings.TrimPrefix(key, prefix)
			var uid int64
			if _, err := fmt.Sscanf(uidStr, "%d", &uid); err == nil {
				result[uid] = append([]string(nil), cmds...)
			}
		}
	}
	return result
}

// ValidCommands lists all commands that can be granted via /approve
var ValidCommands = []string{"summary", "skill", "switch", "list_api"}

func IsValidCommand(cmd string) bool {
	for _, c := range ValidCommands {
		if c == cmd {
			return true
		}
	}
	return false
}
