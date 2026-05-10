package musicsort

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dhowden/tag"
)

// FileMetadata holds extracted metadata for an audio file. AlbumArtist and
// Genre are populated for layouts that need them (albumartist-album-year,
// genre-artist-album-year). TrackNumber and Disc* are used by the
// track-number filename prefix.
type FileMetadata struct {
	Artist      string
	AlbumArtist string
	Album       string
	Title       string
	Genre       string
	Year        string
	TrackNumber int
	DiscNumber  int
	DiscTotal   int
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
		// Prefer AlbumArtist for the primary Artist field (so the default
		// layout's artist folder still groups compilations and feature
		// tracks under their lead). The raw AlbumArtist is also kept so
		// the albumartist-album-year layout can use it directly when
		// chosen.
		meta.AlbumArtist = m.AlbumArtist()
		meta.Artist = m.AlbumArtist()
		if meta.Artist == "" {
			meta.Artist = m.Artist()
		}
		meta.Album = m.Album()
		meta.Title = m.Title()
		meta.Genre = m.Genre()
		if y := m.Year(); y > 0 {
			meta.Year = fmt.Sprintf(" (%d)", y)
		}
		// Track and Disc come back as (number, total). A zero number means
		// the tag wasn't readable, in which case we leave the field at 0
		// so the layout filename rule omits the prefix entirely.
		if n, _ := m.Track(); n > 0 {
			meta.TrackNumber = n
		}
		if n, total := m.Disc(); n > 0 {
			meta.DiscNumber = n
			meta.DiscTotal = total
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
