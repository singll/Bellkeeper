package service

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/singll/bellkeeper/internal/config"
)

// KnowledgeFilesService provides file browsing and management for the knowledge base
type KnowledgeFilesService struct {
	basePath string
	scanDirs []config.ScanDirConfig
}

// TreeNode represents a node in the directory tree
type TreeNode struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	Type     string      `json:"type"` // "dir" or "file"
	Children []*TreeNode `json:"children,omitempty"`
	Size     int64       `json:"size,omitempty"`
	Modified time.Time   `json:"modified,omitempty"`
}

// KnowledgeFileEntry represents file metadata
type KnowledgeFileEntry struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	Type     string    `json:"type"`
	Layer    string    `json:"layer"`
}

// FileContent represents file content for reading
type FileContent struct {
	Path     string    `json:"path"`
	Name     string    `json:"name"`
	Content  string    `json:"content"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

// FilesStats represents file statistics
type FilesStats struct {
	TotalFiles int64            `json:"total_files"`
	TotalDirs  int64            `json:"total_dirs"`
	TotalSize  int64            `json:"total_size"`
	ByLayer    map[string]int64 `json:"by_layer"`
	ByType     map[string]int64 `json:"by_type"`
}

// NewKnowledgeFilesService creates a new knowledge files service
func NewKnowledgeFilesService(cfg config.KnowledgeConfig) *KnowledgeFilesService {
	return &KnowledgeFilesService{
		basePath: cfg.BasePath,
		scanDirs: cfg.ScanDirs,
	}
}

func (s *KnowledgeFilesService) resolvePath(relativePath string) (string, error) {
	if filepath.IsAbs(relativePath) {
		return "", fmt.Errorf("access denied: absolute paths are not allowed")
	}

	absBase, err := filepath.Abs(s.basePath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve base path: %w", err)
	}
	absFull, err := filepath.Abs(filepath.Join(absBase, filepath.Clean(relativePath)))
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}
	if !isPathWithin(absBase, absFull) {
		return "", fmt.Errorf("access denied: path outside base directory")
	}

	realBase, err := filepath.EvalSymlinks(absBase)
	if err != nil {
		return "", fmt.Errorf("failed to resolve base path symlinks: %w", err)
	}
	if realFull, err := filepath.EvalSymlinks(absFull); err == nil && !isPathWithin(realBase, realFull) {
		return "", fmt.Errorf("access denied: path outside base directory")
	}

	return absFull, nil
}

func isPathWithin(base, path string) bool {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

// GetTree returns the directory tree starting from the given path
func (s *KnowledgeFilesService) GetTree(relativePath string) (*TreeNode, error) {
	fullPath, err := s.resolvePath(relativePath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("path not found: %s", relativePath)
		}
		return nil, fmt.Errorf("failed to stat path: %w", err)
	}

	if !info.IsDir() {
		return &TreeNode{
			Name:     filepath.Base(fullPath),
			Path:     relativePath,
			Type:     "file",
			Size:     info.Size(),
			Modified: info.ModTime(),
		}, nil
	}

	return s.buildTreeNode(fullPath, relativePath)
}

// buildTreeNode recursively builds a tree node
func (s *KnowledgeFilesService) buildTreeNode(fullPath, relativePath string) (*TreeNode, error) {
	node := &TreeNode{
		Name: filepath.Base(fullPath),
		Path: relativePath,
		Type: "dir",
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	for _, entry := range entries {
		childRelPath := filepath.Join(relativePath, entry.Name())
		childFullPath := filepath.Join(fullPath, entry.Name())

		if entry.IsDir() {
			// Don't recurse too deep, limit to 3 levels
			depth := strings.Count(childRelPath, string(filepath.Separator))
			if depth < 3 {
				childNode, err := s.buildTreeNode(childFullPath, childRelPath)
				if err == nil {
					node.Children = append(node.Children, childNode)
				}
			} else {
				// Just add a simplified dir node for deep directories
				node.Children = append(node.Children, &TreeNode{
					Name: entry.Name(),
					Path: childRelPath,
					Type: "dir",
				})
			}
		} else {
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if ext == ".md" || ext == ".txt" {
				info, _ := entry.Info()
				node.Children = append(node.Children, &TreeNode{
					Name:     entry.Name(),
					Path:     childRelPath,
					Type:     "file",
					Size:     info.Size(),
					Modified: info.ModTime(),
				})
			}
		}
	}

	// Sort: directories first, then files, alphabetically
	sort.Slice(node.Children, func(i, j int) bool {
		if node.Children[i].Type != node.Children[j].Type {
			return node.Children[i].Type == "dir"
		}
		return node.Children[i].Name < node.Children[j].Name
	})

	return node, nil
}

// ListFiles returns a list of files in the given directory
func (s *KnowledgeFilesService) ListFiles(relativePath string, layer string) ([]KnowledgeFileEntry, error) {
	fullPath, err := s.resolvePath(relativePath)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var files []KnowledgeFileEntry
	layerMap := s.getLayerMap()

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".md" && ext != ".txt" {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		filePath := filepath.Join(relativePath, entry.Name())
		fileLayer := layerMap[filePath]
		if layer != "" && fileLayer != layer {
			continue
		}

		files = append(files, KnowledgeFileEntry{
			Name:     entry.Name(),
			Path:     filePath,
			Size:     info.Size(),
			Modified: info.ModTime(),
			Type:     strings.TrimPrefix(ext, "."),
			Layer:    fileLayer,
		})
	}

	// Sort by modified time descending
	sort.Slice(files, func(i, j int) bool {
		return files[i].Modified.After(files[j].Modified)
	})

	return files, nil
}

// ReadFile returns the content of a file
func (s *KnowledgeFilesService) ReadFile(relativePath string) (*FileContent, error) {
	fullPath, err := s.resolvePath(relativePath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, fmt.Errorf("file not found: %s", relativePath)
	}

	if info.IsDir() {
		return nil, fmt.Errorf("cannot read directory: %s", relativePath)
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return &FileContent{
		Path:     relativePath,
		Name:     info.Name(),
		Content:  string(content),
		Size:     info.Size(),
		Modified: info.ModTime(),
	}, nil
}

// GetStats returns statistics about the knowledge files
func (s *KnowledgeFilesService) GetStats() (*FilesStats, error) {
	stats := &FilesStats{
		ByLayer: make(map[string]int64),
		ByType:  make(map[string]int64),
	}

	layerMap := s.getLayerMap()

	for _, dir := range s.scanDirs {
		fullPath := filepath.Join(s.basePath, dir.Path)
		s.walkDir(fullPath, dir.Path, dir.Layer, stats, layerMap)
	}

	return stats, nil
}

// walkDir recursively walks a directory and collects stats
func (s *KnowledgeFilesService) walkDir(fullPath, relPath, layer string, stats *FilesStats, layerMap map[string]string) {
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return
	}

	for _, entry := range entries {
		childRelPath := filepath.Join(relPath, entry.Name())
		childFullPath := filepath.Join(fullPath, entry.Name())

		if entry.IsDir() {
			stats.TotalDirs++
			s.walkDir(childFullPath, childRelPath, layer, stats, layerMap)
		} else {
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if ext == ".md" || ext == ".txt" {
				stats.TotalFiles++
				info, _ := entry.Info()
				stats.TotalSize += info.Size()

				// Count by layer
				actualLayer := layerMap[childRelPath]
				if actualLayer == "" {
					actualLayer = layer
				}
				stats.ByLayer[actualLayer]++

				// Count by type
				stats.ByType[ext]++
			}
		}
	}
}

// getLayerMap builds a map of relative paths to their layers
func (s *KnowledgeFilesService) getLayerMap() map[string]string {
	layerMap := make(map[string]string)
	for _, dir := range s.scanDirs {
		fullPath := filepath.Join(s.basePath, dir.Path)
		s.mapDirLayers(fullPath, dir.Path, dir.Layer, layerMap)
	}
	return layerMap
}

// mapDirLayers recursively maps all files in a directory to their layer
func (s *KnowledgeFilesService) mapDirLayers(fullPath, relPath, layer string, layerMap map[string]string) {
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return
	}

	for _, entry := range entries {
		childRelPath := filepath.Join(relPath, entry.Name())
		childFullPath := filepath.Join(fullPath, entry.Name())

		if entry.IsDir() {
			s.mapDirLayers(childFullPath, childRelPath, layer, layerMap)
		} else {
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if ext == ".md" || ext == ".txt" {
				layerMap[childRelPath] = layer
			}
		}
	}
}

// SearchFiles searches files by name pattern
func (s *KnowledgeFilesService) SearchFiles(pattern string) ([]KnowledgeFileEntry, error) {
	var results []KnowledgeFileEntry
	layerMap := s.getLayerMap()

	for _, dir := range s.scanDirs {
		fullPath := filepath.Join(s.basePath, dir.Path)
		s.searchDir(fullPath, dir.Path, dir.Layer, pattern, &results, layerMap)
	}

	// Sort by modified time descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Modified.After(results[j].Modified)
	})

	return results, nil
}

// searchDir recursively searches for files matching the pattern
func (s *KnowledgeFilesService) searchDir(fullPath, relPath, layer, pattern string, results *[]KnowledgeFileEntry, layerMap map[string]string) {
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return
	}

	for _, entry := range entries {
		childRelPath := filepath.Join(relPath, entry.Name())
		childFullPath := filepath.Join(fullPath, entry.Name())

		if entry.IsDir() {
			s.searchDir(childFullPath, childRelPath, layer, pattern, results, layerMap)
		} else {
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if ext == ".md" || ext == ".txt" {
				// Case-insensitive pattern matching
				if strings.Contains(strings.ToLower(entry.Name()), strings.ToLower(pattern)) {
					info, _ := entry.Info()
					actualLayer := layerMap[childRelPath]
					if actualLayer == "" {
						actualLayer = layer
					}
					*results = append(*results, KnowledgeFileEntry{
						Name:     entry.Name(),
						Path:     childRelPath,
						Size:     info.Size(),
						Modified: info.ModTime(),
						Type:     strings.TrimPrefix(ext, "."),
						Layer:    actualLayer,
					})
				}
			}
		}
	}
}
