package spotifym3u

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"musicsort/internal/audio"
	"musicsort/internal/clioutput"

	"github.com/dhowden/tag"
)

type localTrack struct {
	Path         string
	Title        string
	Artist       string
	Album        string
	Filename     string
	NormTitle    string
	NormArtist   string
	NormAlbum    string
	NormFilename string
}

type TrackIndex struct {
	rootDir     string
	exactMatch  map[string][]*localTrack
	titleArtist map[string][]*localTrack
	titleOnly   map[string][]*localTrack
	allTracks   []*localTrack
}

func BuildIndex(rootDir string, recursive, verbose bool) (*TrackIndex, error) {
	index := &TrackIndex{
		rootDir:     rootDir,
		exactMatch:  make(map[string][]*localTrack),
		titleArtist: make(map[string][]*localTrack),
		titleOnly:   make(map[string][]*localTrack),
	}

	processed := 0
	walkFn := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if !recursive && path != rootDir {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !audio.SupportedExtensions[ext] {
			return nil
		}

		track, err := buildTrack(rootDir, path)
		if err != nil {
			return nil
		}

		processed++
		if verbose {
			rel, _ := filepath.Rel(rootDir, path)
			clioutput.InfoLine("%s %s", clioutput.Label("INDEX", clioutput.Cyan), filepath.ToSlash(rel))
		} else {
			clioutput.ProgressLine("Indexing %d files...", processed)
		}

		index.allTracks = append(index.allTracks, track)
		index.exactMatch[buildKey(track.NormTitle, track.NormArtist, track.NormAlbum)] = append(index.exactMatch[buildKey(track.NormTitle, track.NormArtist, track.NormAlbum)], track)
		index.titleArtist[buildKey(track.NormTitle, track.NormArtist, "")] = append(index.titleArtist[buildKey(track.NormTitle, track.NormArtist, "")], track)
		index.titleOnly[track.NormTitle] = append(index.titleOnly[track.NormTitle], track)
		return nil
	}

	if err := filepath.WalkDir(rootDir, walkFn); err != nil {
		return nil, fmt.Errorf("walk source directory: %w", err)
	}

	if !verbose {
		fmt.Println()
	}

	if verbose {
		clioutput.InfoLine("Indexed %d tracks", processed)
	}

	return index, nil
}

func buildTrack(rootDir, path string) (*localTrack, error) {
	rel, err := filepath.Rel(rootDir, path)
	if err != nil {
		return nil, err
	}
	rel = filepath.ToSlash(rel)

	track := &localTrack{
		Path:         rel,
		Filename:     filepath.Base(path),
		NormFilename: NormalizeForMatch(filepath.Base(path)),
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	metadata, err := tag.ReadFrom(f)
	if err == nil {
		track.Artist = metadata.AlbumArtist()
		if track.Artist == "" {
			track.Artist = metadata.Artist()
		}
		track.Album = metadata.Album()
		track.Title = metadata.Title()
	}

	if track.Title == "" {
		track.Title = strings.TrimSuffix(track.Filename, filepath.Ext(track.Filename))
	}
	if track.Artist == "" {
		track.Artist = "Unknown Artist"
	}
	if track.Album == "" {
		track.Album = "Unknown Album"
	}

	track.NormTitle = NormalizeForMatch(track.Title)
	track.NormArtist = NormalizeForMatch(track.Artist)
	track.NormAlbum = NormalizeForMatch(track.Album)

	return track, nil
}
