package musicsort

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dhowden/tag"
)

// FileMetadata holds extracted metadata for an audio file.
type FileMetadata struct {
	Artist string
	Album  string
	Title  string
	Year   string
}

// ReadFileMetadata reads metadata from an audio file using the tag library.
func ReadFileMetadata(path string) (FileMetadata, error) {
	f, err := os.Open(path)
	if err != nil {
		return FileMetadata{}, err
	}
	defer f.Close()

	var meta FileMetadata

	m, err := tag.ReadFrom(f)
	if err == nil {
		// Prefer AlbumArtist if available to handle "Various Artists" or group features
		meta.Artist = m.AlbumArtist()
		if meta.Artist == "" {
			meta.Artist = m.Artist()
		}
		meta.Album = m.Album()
		meta.Title = m.Title()
		if y := m.Year(); y > 0 {
			meta.Year = fmt.Sprintf(" (%d)", y)
		}
	}

	// Fallback to filename if no title found
	if meta.Title == "" {
		meta.Title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}

	// Apply defaults for missing values
	if meta.Artist == "" {
		meta.Artist = "Unknown Artist"
	}
	if meta.Album == "" {
		meta.Album = "Unknown Album"
	}

	return meta, nil
}
