package musicmatch

import (
	"reflect"
	"sync"
	"testing"
)

func TestNormalizeForMatch(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"  Beyoncé - Halo (Live)!!! ", "beyonce halo"},
		{"Beyoncé", "beyonce"},
		{"Beyonce", "beyonce"},
		{"Halo", "halo"},
		{"Halo (Live)", "halo"},
		{"Halo - Live", "halo"},
		{"Halo - 2010 Remaster", "halo"},
		{"Halo - Single Version", "halo"},
		{"Song (Original Mix)", "song"},
		{"Song (Bonus Track)", "song"},
		{"Song (feat. Artist)", "song"},
		{"Song [Live]", "song"},
		{"Track ft. Someone", "track"},
		{"Title - featuring Guest", "title"},
		{"Remix (Remix)", "remix"},
		{"Get Lucky - Radio Edit", "get lucky"},
		{"Levels - Original Mix", "levels"},
		{"   Multiple   Spaces   ", "multiple spaces"},
		// Common-noun song titles must NOT be over-stripped just because a
		// keyword like "single" or "original" appears after " - ".
		{"Beyonce - Single Ladies", "beyonce single ladies"},
		{"Artist - Original Sin", "artist original sin"},
		{"Title - Bonus Beats", "title bonus beats"},
		// Patterns lifted directly from the user's Exportify CSVs.
		{"No Quarter - Remaster", "no quarter"},
		{"Reception - Remastered 1993", "reception"},
		{"Cuff Link - Remastered 1993", "cuff link"},
		{"Ozan Koukle - Remastered", "ozan koukle"},
		{"Too Rolling Stoned - 2007 Remaster", "too rolling stoned"},
		{"Bridge of Sighs - 2007 Remaster", "bridge of sighs"},
		{"Custard Pie - Remaster", "custard pie"},
		{"Going to California - 1990 Remaster", "going to california"},
		{"Sidetracked Soundtrack - Unreleased Demo", "sidetracked soundtrack"},
		{"Children of the Moon (feat. Tame Impala)", "children of the moon"},
		{"All The Words We Don't Say - Omari Jazz Remix", "all the words we don t say"},
		{"D'yer Mak'er - Remaster", "d yer mak er"},
		// Lowercase / atypical formatting from the user's CSV.
		{"im sad that my grandma will die - demo", "im sad that my grandma will die"},
		{"im sad that my grandma will die (demo)", "im sad that my grandma will die"},
		// Album-name suffixes that should collapse to the bare album.
		{"Houses of the Holy (Remaster)", "houses of the holy"},
		{"Houses of the Holy (Deluxe Edition)", "houses of the holy"},
		{"Idlewild South (Deluxe Edition Remastered)", "idlewild south"},
		{"London Town (Expanded Edition)", "london town"},
		{"Aeroplane Flies High (Deluxe Edition)", "aeroplane flies high"},
		{"Lonerism (10 Year Anniversary Edition / Unreleased Demos)", "lonerism"},
		{"Mood Variant (The Remixes)", "mood variant"},
		{"Bridge Of Sighs (2007 Remaster)", "bridge of sighs"},
		{"Black Rhapsody (Instrumental)", "black rhapsody"},
		{"Mr. Wonderful (Deluxe)", "mr wonderful"},
		{"Paranoid (Remaster)", "paranoid"},
		// Parens with non-keyword content must NOT be stripped.
		{"Pip Paine (Pay The £5000 You Owe)", "pip paine pay the 5000 you owe"},
		{"Her And I (Slow Jam 2)", "her and i slow jam 2"},
		{"Main Theme From Trouble Man - 2", "main theme from trouble man 2"},
		// Multi-dash titles: only the trailing version annotation is removed.
		// "Rat Salad - 2012 - Remaster" leaves "rat salad 2012" which still
		// title-substring-matches a local "Rat Salad" file.
		{"Rat Salad - 2012 - Remaster", "rat salad 2012"},
		// Diacritic and unicode-punctuation equivalence.
		{"Bahia Dreamin\u2019", "bahia dreamin"},
		{"Gianni\u00b4s Humble", "gianni s humble"},
		{"Blue Öyster Cult", "blue oyster cult"},
	}
	for _, tt := range tests {
		got := NormalizeForMatch(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeForMatch(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeFoldsDiacritics(t *testing.T) {
	if a, b := NormalizeForMatch("Beyoncé"), NormalizeForMatch("Beyonce"); a != b {
		t.Fatalf("expected diacritic-folded equality, got %q vs %q", a, b)
	}
	if a, b := NormalizeForMatch("Sigur Rós"), NormalizeForMatch("Sigur Ros"); a != b {
		t.Fatalf("expected diacritic-folded equality, got %q vs %q", a, b)
	}
	if a, b := NormalizeForMatch("Mötley Crüe"), NormalizeForMatch("Motley Crue"); a != b {
		t.Fatalf("expected diacritic-folded equality, got %q vs %q", a, b)
	}
}

// TestNormalizeConcurrent regression-tests concurrent calls to
// NormalizeForMatch / PrimaryArtistForFolder. A shared transform.Chain used
// with transform.String across goroutines corrupts internal buffers and
// panics (slice bounds out of range).
func TestNormalizeConcurrent(t *testing.T) {
	const workers = 64
	const iters = 200
	inputs := []string{
		"Beyoncé",
		"Mötley Crüe",
		"$uicideboy$",
		"Sigur Rós",
		"Various Artists ft. Guest",
		"$not",
	}
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range iters {
				for _, in := range inputs {
					_ = NormalizeForMatch(in)
					_ = PrimaryArtistForFolder(in)
				}
			}
		}()
	}
	wg.Wait()
}

func TestSplitArtists(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"  ;  ", nil},
		{"Artist1", []string{"Artist1"}},
		// Spotify Exportify joins multi-artist tracks with `;`.
		{"Artist1; Artist2", []string{"Artist1", "Artist2"}},
		{"Artist1;Artist2; Artist3", []string{"Artist1", "Artist2", "Artist3"}},
		{"Beyoncé; Beyonce", []string{"Beyoncé"}},
		// Whitespace-bounded word separators split.
		{"Daft Punk feat. Pharrell Williams", []string{"Daft Punk", "Pharrell Williams"}},
		{"Daft Punk ft. Pharrell", []string{"Daft Punk", "Pharrell"}},
		{"Beyoncé featuring Jay-Z", []string{"Beyoncé", "Jay-Z"}},
		{"Beyoncé with Jay-Z", []string{"Beyoncé", "Jay-Z"}},
		{"Artist1 vs Artist2", []string{"Artist1", "Artist2"}},
		// Real band names from the user's CSV: ampersand, comma, slash, plus
		// must NOT trigger a split. These are single artists.
		{"King Gizzard & The Lizard Wizard", []string{"King Gizzard & The Lizard Wizard"}},
		{"Hall & Oates", []string{"Hall & Oates"}},
		{"Daryl Hall & John Oates", []string{"Daryl Hall & John Oates"}},
		{"Crosby, Stills, Nash & Young", []string{"Crosby, Stills, Nash & Young"}},
		{"Speed, Glue & Shinki", []string{"Speed, Glue & Shinki"}},
		{"30/70", []string{"30/70"}},
		{"Masashi Kitamura + Phonogenix", []string{"Masashi Kitamura + Phonogenix"}},
		// Compound: band-with-ampersand collaborating with another via `;`.
		{"King Gizzard & The Lizard Wizard;Mild High Club",
			[]string{"King Gizzard & The Lizard Wizard", "Mild High Club"}},
	}
	for _, tt := range tests {
		got := SplitArtists(tt.input)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("SplitArtists(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestPrimaryArtistForFolder(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"   ", ""},
		{"$uicideboy$", "$uicideboy$"},
		// The signature win: artist tags joined by `/` survive Sanitize as
		// "_GERM" today. PrimaryArtistForFolder splits on `/` so the folder
		// gets just the leading artist.
		{"$uicideboy$/GERM", "$uicideboy$"},
		{"Artist1/Artist2/Artist3", "Artist1"},
		// `;`-joined Spotify-style multi-artist still picks the lead.
		{"Beyoncé; Jay-Z", "Beyoncé"},
		// Names containing `&`, `,`, or `+` must stay intact (these are
		// real band names, not multi-artist joins).
		{"King Gizzard & The Lizard Wizard", "King Gizzard & The Lizard Wizard"},
		{"Crosby, Stills, Nash & Young", "Crosby, Stills, Nash & Young"},
		{"Masashi Kitamura + Phonogenix", "Masashi Kitamura + Phonogenix"},
		// `30/70` is a real artist name that contains `/`. We accept this
		// limitation: PrimaryArtistForFolder will pick "30" because the
		// folder path can't safely contain a `/` either way.
		{"30/70", "30"},
		// feat./ft./featuring drops the trailing collaborator.
		{"Daft Punk feat. Pharrell", "Daft Punk"},
	}
	for _, tt := range tests {
		got := PrimaryArtistForFolder(tt.input)
		if got != tt.want {
			t.Errorf("PrimaryArtistForFolder(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestWordAlignedSubstring covers the helper used by both the title-substring
// and filename fallbacks.
func TestWordAlignedSubstring(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		// Two-character titles like "as" must NEVER match longer strings,
		// because a chunk like "ask me no questions" character-contains "as"
		// and that's how Stevie Wonder's "As" used to wrongly match
		// "B.B. King;Leon Russell - Ask Me No Questions".
		{"two-char as in askmenoquestions", "ask me no questions", "as", false},
		{"two-char as in asshtonpark", "asshtonpark", "as", false},
		{"three-char run in running", "running on faith", "run", false},
		// Four-character word-aligned substrings (e.g. "halo") are the
		// minimum length that catches legitimate short song titles.
		{"halo in beyonce halo mp3", "beyonce halo mp3", "halo", true},
		{"halo not word-aligned in halothane", "halothane", "halo", false},
		{"single ladies in single ladies put a ring on it", "single ladies put a ring on it", "single ladies", true},
		// Substring direction is symmetric.
		{"haystack contained in needle", "halo", "beyonce halo mp3", true},
		// Trouble-Man bug: "trouble" IS word-aligned in
		// "main theme from trouble man 2", so the helper alone returns
		// true. The artist-shared gate at the call site is what prevents
		// the actual false positive (Marvin Gaye vs Jim Guthrie).
		{"trouble word-aligned in trouble man", "main theme from trouble man 2", "trouble", true},
		// Empty / equal cases.
		{"empty needle", "anything", "", false},
		{"identical strings >= min length", "halo", "halo", true},
		{"identical strings below min length", "as", "as", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WordAlignedSubstring(tt.a, tt.b); got != tt.want {
				t.Fatalf("WordAlignedSubstring(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// TestSharesArtistFuzzy is a unit test for the artist substring fallback.
// The fuzzy path must catch real-world inconsistencies (Wings ⊂ Paul
// McCartney And Wings, Emancipator ⊂ Emancipator Cloudchord) without
// misfiring for common short tokens (The, And, King).
func TestSharesArtistFuzzy(t *testing.T) {
	cases := []struct {
		name        string
		localNorms  []string
		entry       []string
		wantMatched bool
	}{
		// Strict-equality cases (regression coverage that fuzzy path
		// hasn't broken the common case).
		{"strict equality", []string{"beyonce"}, []string{"Beyoncé"}, true},
		{"strict no match", []string{"sza"}, []string{"Beyoncé"}, false},
		{"empty local", nil, []string{"Beyoncé"}, false},
		{"empty entry", []string{"beyonce"}, nil, false},

		// Fuzzy cases that we *want* to catch.
		{
			name:        "Wings ⊂ Paul McCartney And Wings",
			localNorms:  []string{"paul mccartney and wings"},
			entry:       []string{"Wings"},
			wantMatched: true,
		},
		{
			name:        "Emancipator ⊂ Emancipator Cloudchord (& stripped)",
			localNorms:  []string{"emancipator cloudchord"},
			entry:       []string{"Emancipator", "Cloudchord"},
			wantMatched: true,
		},
		{
			name:        "Alice Coltrane ⊂ Alice Coltrane Pharoah Sanders",
			localNorms:  []string{"various artists", "alice coltrane pharoah sanders"},
			entry:       []string{"Alice Coltrane", "Pharoah Sanders"},
			wantMatched: true,
		},
		{
			name:        "Frank Zappa ⊂ Frank Zappa And The Mothers",
			localNorms:  []string{"frank zappa and the mothers"},
			entry:       []string{"Frank Zappa"},
			wantMatched: true,
		},

		// Fuzzy cases that we *do not* want to catch — short tokens
		// would otherwise spuriously merge unrelated artists.
		{
			name:        "the (3 chars) does not match The Beach Boys",
			localNorms:  []string{"the beach boys"},
			entry:       []string{"The"},
			wantMatched: false,
		},
		{
			name:        "and (3 chars) does not match Paul McCartney And Wings",
			localNorms:  []string{"paul mccartney and wings"},
			entry:       []string{"And"},
			wantMatched: false,
		},
		{
			name:        "king (4 chars) does not match King Gizzard & The Lizard Wizard",
			localNorms:  []string{"king gizzard the lizard wizard"},
			entry:       []string{"King"},
			wantMatched: false,
		},
		{
			name:        "non-word-aligned substring rejected",
			localNorms:  []string{"halothane theory"},
			entry:       []string{"Halo"},
			wantMatched: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SharesArtist(tc.localNorms, tc.entry)
			if got != tc.wantMatched {
				t.Fatalf("SharesArtist(%v, %v) = %v, want %v", tc.localNorms, tc.entry, got, tc.wantMatched)
			}
		})
	}
}

func TestAlbumYearSuffix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Faces (2014)", "Faces"},
		{"Album", "Album"},
		{"Album (1999)  ", "Album"},
		{"Album (2024) extra", "Album (2024) extra"},
		{"Random Access Memories", "Random Access Memories"},
	}
	for _, tt := range tests {
		got := AlbumYearSuffix.ReplaceAllString(tt.input, "")
		if got != tt.want {
			t.Errorf("AlbumYearSuffix(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBuildKey(t *testing.T) {
	if k := BuildKey("a", "b", "c"); k != "a\x1fb\x1fc" {
		t.Fatalf("BuildKey produced %q", k)
	}
	if k := BuildKey("a", "", "c"); k != "a\x1f\x1fc" {
		t.Fatalf("BuildKey with empty middle produced %q", k)
	}
}

func TestJaccard(t *testing.T) {
	a := TokenSet("alpha beta gamma")
	b := TokenSet("beta gamma delta")
	got := Jaccard(a, b)
	want := 2.0 / 4.0 // |{beta, gamma}| / |{alpha, beta, gamma, delta}|
	if got != want {
		t.Fatalf("Jaccard = %v, want %v", got, want)
	}
	if Jaccard(nil, b) != 0 {
		t.Fatalf("expected 0 for empty input")
	}
	if Jaccard(a, a) != 1.0 {
		t.Fatalf("expected 1 for identical sets")
	}
}
