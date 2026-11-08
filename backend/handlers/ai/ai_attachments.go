package ai

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"hephaestus-backend/lib/pentest"
	"hephaestus-backend/pkg/config"

	"github.com/google/uuid"
)

// persistMessageAttachments writes arbitrary uploads under the conversation
// workspace using a generated directory and generated file names. The client
// supplied name is display metadata only; it is never joined as a path.
func persistMessageAttachments(conversationID string, files []pentest.MessageAttachmentUpload) ([]string, error) {
	return persistMessageAttachmentsAt(config.GetDataDir(), conversationID, files)
}

func persistMessageAttachmentsAt(dataDir, conversationID string, files []pentest.MessageAttachmentUpload) ([]string, error) {
	if len(files) == 0 {
		return nil, nil
	}
	if conversationID == "" || filepath.Base(conversationID) != conversationID {
		return nil, fmt.Errorf("会话 ID 无效")
	}
	root, err := os.OpenRoot(dataDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	batch := uuid.NewString()
	dir := filepath.Join("ai-pentest", "sessions", conversationID, "attachments", batch)
	if err = root.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(files))
	for _, file := range files {
		name, nameErr := pentest.SafeAttachmentName(file.Name, "attachment")
		if nameErr != nil {
			return nil, nameErr
		}
		stored := file.ID + "-" + name
		full := filepath.Join(dir, stored)
		handle, openErr := root.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if openErr != nil {
			return nil, openErr
		}
		_, writeErr := handle.Write(file.Data)
		if closeErr := handle.Close(); writeErr == nil {
			writeErr = closeErr
		}
		if writeErr != nil {
			return nil, writeErr
		}
		// Return the path relative to the conversation workspace, which is the
		// root the Agent uses for read/exec tools.
		paths = append(paths, filepath.ToSlash(filepath.Join("attachments", batch, stored)))
	}
	return paths, nil
}

func messageWithAttachmentManifest(content string, conversationID string, files []pentest.MessageAttachmentUpload) (string, error) {
	paths, err := persistMessageAttachments(conversationID, files)
	if err != nil {
		return content, err
	}
	if len(paths) == 0 {
		return content, nil
	}
	lines := make([]string, 0, len(paths))
	for index, file := range files {
		if file.Image {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s (%s, %d bytes) -> %s", file.Name, file.ContentType, file.Size, paths[index]))
	}
	if len(lines) == 0 {
		return content, nil
	}
	manifest := "[附件]\n" + strings.Join(lines, "\n")
	if strings.TrimSpace(content) == "" {
		return manifest, nil
	}
	return content + "\n\n" + manifest, nil
}
