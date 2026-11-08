package ai

import (
	"fmt"
	"strconv"
	"strings"
)

// normalizePentestTaskReference accepts the canonical taskId plus the legacy
// display form "<taskId>/v<version>". Older evidence prompts rendered task and
// version as one token, which models could copy back into the taskId field.
func normalizePentestTaskReference(rawTaskID string, explicitVersion int) (string, int, error) {
	taskID := strings.TrimSpace(rawTaskID)
	if taskID == "" {
		return "", explicitVersion, fmt.Errorf("taskId 必填")
	}
	if explicitVersion < 0 {
		return "", explicitVersion, fmt.Errorf("version 不能为负数")
	}

	if marker := strings.LastIndex(taskID, "/v"); marker > 0 {
		rawVersion := taskID[marker+2:]
		if rawVersion != "" {
			parsedVersion, err := strconv.Atoi(rawVersion)
			if err == nil && parsedVersion >= 0 {
				taskID = strings.TrimSpace(taskID[:marker])
				if explicitVersion == 0 && parsedVersion > 0 {
					explicitVersion = parsedVersion
				}
			}
		}
	}
	if taskID == "" {
		return "", explicitVersion, fmt.Errorf("taskId 必填")
	}
	return taskID, explicitVersion, nil
}
