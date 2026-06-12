package reporter

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const maxInlineSize = 1 << 20 // 1 MB

// ArtifactEntry holds the metadata and (optionally) the content of an artifact file.
type ArtifactEntry struct {
	RelPath string `json:"rel_path"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
	Content []byte `json:"content,omitempty"` // only for files < 1 MB
}

// OutputPackage bundles all task output for submission to the control plane.
type OutputPackage struct {
	TaskID     string            `json:"task_id"`
	WorkerID   string            `json:"worker_id"`
	Artifacts  []ArtifactEntry   `json:"artifacts"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
	HMACSha256 string            `json:"hmac_sha256"`
}

// Pack collects the listed artifact files (relative paths within workspacePath),
// computes per-file SHA256 checksums, and signs the entire package with HMAC-SHA256.
func Pack(taskID, workerID, workspacePath, hmacSecret string, artifacts []string) (*OutputPackage, error) {
	pkg := &OutputPackage{
		TaskID:   taskID,
		WorkerID: workerID,
		Metadata: map[string]any{},
	}

	for _, relPath := range artifacts {
		// Safety: ensure the path stays within the workspace
		absPath := filepath.Join(workspacePath, relPath)
		cleanWorkspace := filepath.Clean(workspacePath)
		cleanAbs := filepath.Clean(absPath)

		if len(cleanAbs) <= len(cleanWorkspace) || cleanAbs[:len(cleanWorkspace)+1] != cleanWorkspace+string(filepath.Separator) {
			// Also allow exact equality (the workspace dir itself, if needed)
			if cleanAbs != cleanWorkspace {
				return nil, fmt.Errorf("artifact path %q escapes workspace", relPath)
			}
		}

		entry, err := packFile(absPath, relPath)
		if err != nil {
			return nil, fmt.Errorf("pack artifact %q: %w", relPath, err)
		}

		pkg.Artifacts = append(pkg.Artifacts, *entry)
	}

	// Sign the package contents (excluding HMACSha256 itself)
	sig, err := signPackage(pkg, hmacSecret)
	if err != nil {
		return nil, fmt.Errorf("sign package: %w", err)
	}
	pkg.HMACSha256 = sig

	return pkg, nil
}

// packFile reads a single file, computes its SHA256, and returns an ArtifactEntry.
func packFile(absPath, relPath string) (*ArtifactEntry, error) {
	f, err := os.Open(absPath)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", absPath, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w", absPath, err)
	}

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, fmt.Errorf("hash %q: %w", absPath, err)
	}
	checksum := hex.EncodeToString(h.Sum(nil))

	entry := &ArtifactEntry{
		RelPath: relPath,
		Size:    info.Size(),
		SHA256:  checksum,
	}

	// Inline small files
	if info.Size() < maxInlineSize {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("seek %q: %w", absPath, err)
		}
		content, err := io.ReadAll(f)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", absPath, err)
		}
		entry.Content = content
	}

	return entry, nil
}

// signPackage computes an HMAC-SHA256 over the package fields (excluding HMACSha256).
func signPackage(pkg *OutputPackage, secret string) (string, error) {
	// Build the canonical representation for signing
	payload := struct {
		TaskID    string         `json:"task_id"`
		WorkerID  string         `json:"worker_id"`
		Artifacts []ArtifactEntry `json:"artifacts"`
		Metadata  map[string]any  `json:"metadata,omitempty"`
	}{
		TaskID:    pkg.TaskID,
		WorkerID:  pkg.WorkerID,
		Artifacts: pkg.Artifacts,
		Metadata:  pkg.Metadata,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload for signing: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil)), nil
}
