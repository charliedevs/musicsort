package musicsort

import (
	"os"
	"path/filepath"
	"testing"
)

// seedTargetTree creates a fake target directory with the canonical
// "Artist/Album/file.ext" layout. Each (artist, album, file) triple is
// expanded to a real on-disk path with the given file contents so the
// consolidation pass can run against it.
func seedTargetTree(t *testing.T, root string, entries []seedEntry) {
	t.Helper()
	for _, e := range entries {
		dir := filepath.Join(root, e.artist, e.album)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		path := filepath.Join(dir, e.file)
		if err := os.WriteFile(path, []byte(e.content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}

type seedEntry struct {
	artist  string
	album   string
	file    string
	content string
}

// TestTargetIndexBuildsBuckets verifies that BuildTargetIndex collects
// case-fold artist dupes (`$uicideboy$` vs `$UICIDEBOY$`) and edition-
// variant album dupes (`Faces (2014)` vs `FACES (2014)`) into the same
// normalized buckets.
func TestTargetIndexBuildsBuckets(t *testing.T) {
	tmp := t.TempDir()
	seedTargetTree(t, tmp, []seedEntry{
		{"$uicideboy$", "I Want to Die in New Orleans (2018)", "track1.mp3", "a"},
		{"$UICIDEBOY$", "I No Longer Fear the Razor (III) (2016)", "track2.mp3", "b"},
		{"Mac Miller", "Faces (2014)", "01 - Polo Jeans.mp3", "c"},
		{"Mac Miller", "FACES (2014)", "01 - Polo Jeans.mp3", "d"},
	})

	layout, _ := LayoutByName("artist-album-year")
	idx, err := BuildTargetIndex(tmp, layout)
	if err != nil {
		t.Fatalf("BuildTargetIndex: %v", err)
	}

	// Two case-fold artist folders share one normalized key.
	suiBucket := idx.artists["uicideboy"]
	if len(suiBucket) != 2 {
		t.Fatalf("expected 2 case-fold suicideboy folders, got %d (%v)", len(suiBucket), suiBucket)
	}

	// Mac Miller is one artist with two case-fold album dupes under it.
	macBucket := idx.artists["mac miller"]
	if len(macBucket) != 1 {
		t.Fatalf("expected 1 Mac Miller artist folder, got %d", len(macBucket))
	}
	mac := macBucket[0]
	facesBucket := mac.Albums["faces"]
	if len(facesBucket) != 2 {
		t.Fatalf("expected 2 case-fold Faces folders, got %d", len(facesBucket))
	}
}

// TestResolveArtistAndAlbumUseCanonical confirms that when an existing
// folder already covers an incoming file's artist / album, the canonical
// on-disk name is returned (not Sanitize(rawArtist)). This is the
// "$UICIDEBOY$ tagged file should land in the existing $uicideboy$ folder"
// behaviour the README TODO calls for.
func TestResolveArtistAndAlbumUseCanonical(t *testing.T) {
	tmp := t.TempDir()
	seedTargetTree(t, tmp, []seedEntry{
		{"$uicideboy$", "Radical $uicide (2016)", "01.mp3", "x"},
		{"$uicideboy$", "Radical $uicide (2016)", "02.mp3", "y"},
	})

	layout, _ := LayoutByName("artist-album-year")
	idx, err := BuildTargetIndex(tmp, layout)
	if err != nil {
		t.Fatal(err)
	}

	got := idx.ResolveArtist("$UICIDEBOY$")
	if got != "$uicideboy$" {
		t.Fatalf("ResolveArtist(case-variant) = %q, want %q", got, "$uicideboy$")
	}

	got = idx.ResolveAlbum("$UICIDEBOY$", "RADICAL $UICIDE", " (2016)")
	if got != "Radical $uicide (2016)" {
		t.Fatalf("ResolveAlbum = %q, want %q", got, "Radical $uicide (2016)")
	}
}

// TestResolveArtistFallsBackForNewArtist makes sure unknown artists still
// get a usable folder name (Sanitize of the raw tag) when there's no
// existing match. Without this, every incoming file with no on-disk
// equivalent would silently land in "" and break the move.
func TestResolveArtistFallsBackForNewArtist(t *testing.T) {
	tmp := t.TempDir()
	layout, _ := LayoutByName("artist-album-year")
	idx, _ := BuildTargetIndex(tmp, layout)

	got := idx.ResolveArtist("Beyoncé")
	if got != "Beyoncé" {
		t.Fatalf("ResolveArtist new artist = %q, want %q", got, "Beyoncé")
	}
}

// TestConsolidateArtistDupes covers the end-to-end consolidate path: two
// case-fold artist folders end up merged into one, and the loser folder
// is removed.
func TestConsolidateArtistDupes(t *testing.T) {
	tmp := t.TempDir()
	seedTargetTree(t, tmp, []seedEntry{
		// Winner: 3 files, longer name.
		{"$uicideboy$", "Radical $uicide (2016)", "01 - Track A.mp3", "a"},
		{"$uicideboy$", "Radical $uicide (2016)", "02 - Track B.mp3", "b"},
		{"$uicideboy$", "Radical $uicide (2016)", "03 - Track C.mp3", "c"},
		// Loser: 1 file.
		{"$UICIDEBOY$", "I No Longer Fear (III) (2016)", "01 - Track D.mp3", "d"},
	})

	layout, _ := LayoutByName("artist-album-year")
	idx, err := BuildTargetIndex(tmp, layout)
	if err != nil {
		t.Fatal(err)
	}
	org := &TrackOrganizer{
		index:     idx,
		layout:    layout,
		targetDir: tmp,
	}
	if err := org.consolidateExisting(); err != nil {
		t.Fatalf("consolidateExisting: %v", err)
	}

	if org.result.Consolidated == 0 {
		t.Fatalf("expected at least 1 consolidated folder, got 0")
	}
	if _, err := os.Stat(filepath.Join(tmp, "$UICIDEBOY$")); !os.IsNotExist(err) {
		t.Fatalf("loser folder $UICIDEBOY$ should have been removed, err=%v", err)
	}
	movedFile := filepath.Join(tmp, "$uicideboy$", "I No Longer Fear (III) (2016)", "01 - Track D.mp3")
	if _, err := os.Stat(movedFile); err != nil {
		t.Fatalf("expected loser file at %s after merge, err=%v", movedFile, err)
	}
}

// TestConsolidateAlbumDupesWithCollision exercises the collision policy:
// two same-named tracks under case-fold album folders. Identical bytes
// must dedupe (loser file deleted, no rename), differing bytes must
// rename ("(2)").
func TestConsolidateAlbumDupesWithCollision(t *testing.T) {
	tmp := t.TempDir()
	// Identical-content collision.
	seedTargetTree(t, tmp, []seedEntry{
		{"Mac Miller", "Faces (2014)", "01 - Polo Jeans.mp3", "same"},
		{"Mac Miller", "FACES (2014)", "01 - Polo Jeans.mp3", "same"},
	})
	// Differing-content collision (separate file in the loser).
	seedTargetTree(t, tmp, []seedEntry{
		{"Mac Miller", "FACES (2014)", "02 - Other.mp3", "loserbytes"},
		{"Mac Miller", "Faces (2014)", "02 - Other.mp3", "winnerbytes"},
	})

	layout, _ := LayoutByName("artist-album-year")
	idx, err := BuildTargetIndex(tmp, layout)
	if err != nil {
		t.Fatal(err)
	}
	org := &TrackOrganizer{index: idx, layout: layout, targetDir: tmp}
	if err := org.consolidateExisting(); err != nil {
		t.Fatalf("consolidateExisting: %v", err)
	}

	winnerDir := filepath.Join(tmp, "Mac Miller", "Faces (2014)")
	loserDir := filepath.Join(tmp, "Mac Miller", "FACES (2014)")
	if _, err := os.Stat(loserDir); !os.IsNotExist(err) {
		t.Fatalf("loser dir should be gone, err=%v", err)
	}
	// Identical-content dupe: only one file at "01 - Polo Jeans.mp3".
	if _, err := os.Stat(filepath.Join(winnerDir, "01 - Polo Jeans.mp3")); err != nil {
		t.Fatalf("expected winner's polo jeans to survive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(winnerDir, "01 - Polo Jeans (2).mp3")); !os.IsNotExist(err) {
		t.Fatalf("identical bytes should not have produced a (2) rename")
	}
	// Differing-bytes collision: both files survive, the loser was renamed.
	if _, err := os.Stat(filepath.Join(winnerDir, "02 - Other.mp3")); err != nil {
		t.Fatalf("expected winner's 02 - Other.mp3 to survive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(winnerDir, "02 - Other (2).mp3")); err != nil {
		t.Fatalf("differing bytes should have been renamed to (2): %v", err)
	}
	if org.result.RenamedOnMerge != 1 {
		t.Fatalf("RenamedOnMerge = %d, want 1", org.result.RenamedOnMerge)
	}
}

// TestConsolidateDryRun verifies the planning-only mode doesn't move
// anything but does still walk the dupes.
func TestConsolidateDryRun(t *testing.T) {
	tmp := t.TempDir()
	seedTargetTree(t, tmp, []seedEntry{
		{"$uicideboy$", "Album A (2016)", "01.mp3", "a"},
		{"$UICIDEBOY$", "Album B (2017)", "01.mp3", "b"},
	})

	layout, _ := LayoutByName("artist-album-year")
	idx, _ := BuildTargetIndex(tmp, layout)
	org := &TrackOrganizer{index: idx, layout: layout, targetDir: tmp, dryRun: true}
	if err := org.consolidateExisting(); err != nil {
		t.Fatal(err)
	}

	// Dry run records intent in the counter but doesn't touch disk.
	if org.result.Consolidated == 0 {
		t.Fatalf("dry-run should still count planned consolidations")
	}
	if _, err := os.Stat(filepath.Join(tmp, "$UICIDEBOY$")); err != nil {
		t.Fatalf("dry-run should not remove loser dir, err=%v", err)
	}
}

// TestRunSmokeTest is a higher-level smoke test that walks a fake source
// dir into a fake target with seeded dupes and checks both flow paths:
// (1) consolidation runs and removes case-fold dupes, (2) a new file
// routes into the canonical artist folder rather than creating a new
// case variant.
func TestRunSmokeTest(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "target")
	source := filepath.Join(tmp, "source")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}

	// Seed the target with one canonical artist + one case-fold loser.
	seedTargetTree(t, target, []seedEntry{
		{"$uicideboy$", "Album A (2016)", "01 - Existing.mp3", "a"},
		{"$UICIDEBOY$", "Album B (2017)", "01 - Existing.mp3", "b"},
	})

	// Drop a (tagless) source file. With no tag-readable bytes, the
	// organizer falls back to filename → title and "Unknown Artist" /
	// "Unknown Album", which doesn't exercise the real artist routing.
	// We instead exercise the consolidation half of Run, which is
	// what the README TODO directly addresses.
	sourceTrack := filepath.Join(source, "stray.mp3")
	if err := os.WriteFile(sourceTrack, []byte("not really audio"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		SourceDir: source,
		TargetDir: target,
		Recursive: false,
	}
	if err := Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// After Run, the case-fold loser folder must be gone.
	if _, err := os.Stat(filepath.Join(target, "$UICIDEBOY$")); !os.IsNotExist(err) {
		t.Fatalf("expected case-fold loser folder removed, err=%v", err)
	}
	// The loser's album subdir must now live under the canonical artist.
	merged := filepath.Join(target, "$uicideboy$", "Album B (2017)", "01 - Existing.mp3")
	if _, err := os.Stat(merged); err != nil {
		t.Fatalf("expected loser file to be merged into canonical, err=%v", err)
	}
}

// TestRunRespectsNoConsolidate confirms the opt-out flag preserves the
// pre-existing dupe layout exactly (for users who deliberately want it).
func TestRunRespectsNoConsolidate(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "target")
	source := filepath.Join(tmp, "source")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	seedTargetTree(t, target, []seedEntry{
		{"$uicideboy$", "Album A (2016)", "01.mp3", "a"},
		{"$UICIDEBOY$", "Album B (2017)", "01.mp3", "b"},
	})

	cfg := Config{
		SourceDir:     source,
		TargetDir:     target,
		Recursive:     false,
		NoConsolidate: true,
	}
	if err := Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Both pre-existing folders must still be there.
	for _, p := range []string{"$uicideboy$", "$UICIDEBOY$"} {
		if _, err := os.Stat(filepath.Join(target, p)); err != nil {
			t.Fatalf("expected %s to survive --no-consolidate, err=%v", p, err)
		}
	}
}
