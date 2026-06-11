package chunker

import (
	"strings"
)

// Chunk represents a text chunk with metadata
type Chunk struct {
	Index       int    `json:"index"`
	Text        string `json:"text"`
	HeadingPath string `json:"heading_path"`
	StartPos    int    `json:"start_pos"`
	EndPos      int    `json:"end_pos"`
}

// Config holds chunking configuration
type Config struct {
	TargetChars  int `json:"target_chars"`  // Target chunk size in characters
	OverlapChars int `json:"overlap_chars"` // Overlap between chunks
}

// DefaultConfig returns default chunking configuration
func DefaultConfig() Config {
	return Config{
		TargetChars:  1000,
		OverlapChars: 200,
	}
}

// ChunkMarkdown splits markdown content into chunks
func ChunkMarkdown(content string, cfg Config) []Chunk {
	if cfg.TargetChars <= 0 {
		cfg = DefaultConfig()
	}

	lines := strings.Split(content, "\n")
	var chunks []Chunk
	var currentChunk strings.Builder
	var headingPath string
	chunkIndex := 0
	startPos := 0
	pos := 0

	for _, line := range lines {
		lineLen := len(line) + 1 // +1 for newline

		// Check if this is a heading
		if isHeading(line) {
			// If we have accumulated content, save it as a chunk
			if currentChunk.Len() > 0 {
				text := strings.TrimSpace(currentChunk.String())
				if len(text) > 0 {
					chunks = append(chunks, Chunk{
						Index:       chunkIndex,
						Text:        text,
						HeadingPath: headingPath,
						StartPos:    startPos,
						EndPos:      pos,
					})
					chunkIndex++
				}
				currentChunk.Reset()
				startPos = pos
			}

			// Update heading path
			level, title := parseHeading(line)
			headingPath = updateHeadingPath(headingPath, level, title)
		}

		// Add line to current chunk
		if currentChunk.Len() > 0 {
			currentChunk.WriteString("\n")
		}
		currentChunk.WriteString(line)
		pos += lineLen

		// Check if chunk is large enough
		if currentChunk.Len() >= cfg.TargetChars {
			text := strings.TrimSpace(currentChunk.String())
			if len(text) > 0 {
				chunks = append(chunks, Chunk{
					Index:       chunkIndex,
					Text:        text,
					HeadingPath: headingPath,
					StartPos:    startPos,
					EndPos:      pos,
				})
				chunkIndex++

				// Create overlap
				overlapStart := max(0, currentChunk.Len()-cfg.OverlapChars)
				currentChunk.Reset()
				currentChunk.WriteString(text[overlapStart:])
				startPos = pos - cfg.OverlapChars
			}
		}
	}

	// Add remaining content
	if currentChunk.Len() > 0 {
		text := strings.TrimSpace(currentChunk.String())
		if len(text) > 0 {
			chunks = append(chunks, Chunk{
				Index:       chunkIndex,
				Text:        text,
				HeadingPath: headingPath,
				StartPos:    startPos,
				EndPos:      pos,
			})
		}
	}

	return chunks
}

// isHeading checks if a line is a markdown heading
func isHeading(line string) bool {
	return len(line) > 0 && line[0] == '#' && (len(line) == 1 || line[1] == ' ' || line[1] == '#')
}

// parseHeading extracts level and title from a heading line
func parseHeading(line string) (int, string) {
	level := 0
	for i, ch := range line {
		if ch == '#' {
			level++
		} else if ch == ' ' {
			return level, strings.TrimSpace(line[i:])
		}
	}
	return level, strings.TrimSpace(line[level:])
}

// updateHeadingPath updates the heading breadcrumb path
func updateHeadingPath(currentPath string, level int, title string) string {
	parts := strings.Split(currentPath, " > ")
	if len(parts) >= level {
		parts = parts[:level-1]
	}
	parts = append(parts, title)
	return strings.Join(parts, " > ")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
