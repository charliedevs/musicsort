# musicsort

CLI tools for organizing music files and creating M3U playlists from Exportify CSV exports.

## TODO

- [ ] Maybe actually make the interface and output for both tools consistent with each other
- [ ] Custom organization/directory naming for musicsort

## CLI Tools

### musicsort

Takes music files in directory and organizes into folders.

- Concurrent processing with Go routines
- Native metadata parsing without external dependencies
- Dry-run and safe-skip modes to prevent data loss
- Places into `Artist/Album (Year)/` directory structure

### spotifym3u

Creates a m3u playlist file based on Exportify CSV file and your organized music files.

- Multi-stage matching (title+artist+album, title+artist, title-only, filename)
- Reads Exportify CSV and creates M3U playlists
- Handles incomplete or variant metadata
- Configurable path prefixes for different devices

## Installation

### Quick Install (Linux/macOS)

Ensure [Go 1.20+](https://go.dev/doc/install) is installed, then:

```bash
git clone https://github.com/yourusername/musicsort.git
cd musicsort
make install
```

This builds and installs both `musicsort` and `spotifym3u` to `~/.local/bin/`.

### Build Individual Tools

musicsort only:

```bash
make build-musicsort
```

spotifym3u only:

```bash
make build-spotifym3u
```

### Build Both

```bash
make build
```

Binaries will be created in the current directory.

## Workflow

Below is the process I follow to get my spotify playlists onto my portable audio player. Feel free to try something similar.

> **Note:** Be sure you have the `musicsort` and `spotifym3u` tools installed and in your PATH.

### Step-by-Step Guide

#### 1. Export Playlists from Spotify

Use [Exportify](https://github.com/watsonbox/exportify) to export your Spotify playlists as CSV files:

1. Visit [https://exportify.app/](https://exportify.app/)
2. Click "Get Started" and authenticate with your Spotify account
3. Select the playlists you want to export
4. Click "Export" to download CSV files

**Result:** CSV files containing track information (track name, artist, album, etc.)

#### 2. Use sldl to get music files

Use [sldl](https://github.com/fiso64/sldl) to download the tracks locally:

1. Install sldl: Download from [releases](https://github.com/fiso64/sldl/releases)
2. Configure your credentials in `sldl.conf`. Mine looks like:
   ```
   username = ******
   password = ******
   pref-format = mp3
   fast-search = true
   ```
3. Run sldl with your exported CSV:
   ```bash
   ./sldl "MyPlaylist.csv" -p ~/Downloads/music --name-format "{artist} - {title}"
   ```

**Result:** Downloaded music files in `~/Downloads/music` (unorganized)

> **Note:** Only use this to get music you have the rights to access in your region. This project is unaffiliated with sldl.

#### 3. Organize Your Music Library

Use musicsort to organize files by artist and album:

```bash
musicsort -s ~/Downloads/music -t ~/Music -r
```

This command:

- Scans `~/Downloads/music` recursively
- Reads metadata from each file
- Creates an organized structure: `~/Music/Artist/Album (Year)/`
- Moves files to match the structure

**Result:** Organized music library in `~/Music/`

#### 4. Create M3U Playlists

Use spotifym3u to create M3U playlist files that reference your organized music:

```bash
spotifym3u -csv MyPlaylist.csv -source ~/Music -output MyPlaylist.m3u -target-prefix "mnt/SDCARD/Music" -recursive
```

> **Note:** Make sure `-target-prefix` matches the location where you'll place the music files.

This command:

- Reads your original Exportify CSV
- Searches your organized music library
- Creates an M3U playlist file that references the organized files

**Result:** `MyPlaylist.m3u` that can be loaded in a music player

#### 5. (Optional) Transfer to Portable Device

Copy the organized music and M3U files to your portable device (e.g., SD card for a DAP):

```bash
# Copy organized music folders
cp -r ~/Music/Artist_Name "/mnt/SDCARD/Music/"

# Copy playlist files (check where m3u playlists are stored on your device)
cp *.m3u "/mnt/SDCARD/Playlists/"
```

Load the M3U in a music player to play from the organized library.

## Usage

### musicsort

Organize music files by metadata into an `Artist/Album (Year)/` hierarchy.

```bash
musicsort [OPTIONS]
```

| Flag | Description                              | Default |
| :--- | :--------------------------------------- | :------ |
| -s   | Source directory to scan                 | .       |
| -t   | Target directory for organized files     | .       |
| -r   | Enable recursive search                  | false   |
| -n   | Dry run (preview changes without moving) | false   |

#### Examples

Organize a downloads folder into a music library recursively:

```bash
musicsort -s ~/Downloads -t ~/Music -r
```

Preview changes without moving files:

```bash
musicsort -s ~/Downloads -t ~/Music -r -n
```

### spotifym3u

Create an M3U playlist from a Exportify CSV export.

```bash
spotifym3u -csv PLAYLIST.csv -source ~/Music -output playlist.m3u
```

| Flag           | Description                              | Default      |
| :------------- | :--------------------------------------- | :----------- |
| -csv           | Path to the Exportify CSV file           | required     |
| -source        | Source directory containing music files  | .            |
| -output        | Output M3U playlist file                 | playlist.m3u |
| -target-prefix | Prefix to prepend to each playlist entry | (none)       |
| -recursive     | Scan source directory recursively        | false        |
| -dry-run       | Show match results without writing file  | false        |
| -debug         | Print unmatched track details            | false        |

#### Examples

Create a playlist for a mobile device with a custom prefix:

```bash
spotifym3u -csv MyPlaylist.csv -source ~/Music -output Mobile.m3u -target-prefix /sdcard/Music -recursive
```

Preview matches without writing the file:

```bash
spotifym3u -csv MyPlaylist.csv -source ~/Music -dry-run -debug
```

Export unmatched tracks to debug missing songs:

```bash
spotifym3u -csv MyPlaylist.csv -source ~/Music -debug
```

## Development

### Project Structure

```
musicsort/
├── cmd/                     # Command-line entry points
│   ├── musicsort/          # musicsort CLI
│   └── spotifym3u/         # spotifym3u CLI
├── internal/               # Internal packages (not exported)
│   ├── musicsort/          # musicsort implementation
│   └── spotifym3u/         # spotifym3u implementation
├── Makefile                # Build and installation targets
├── README.md              # This file
└── go.mod / go.sum        # Go module files
```

### Makefile Targets

```bash
make build              # Build both musicsort and spotifym3u
make build-musicsort    # Build only musicsort
make build-spotifym3u   # Build only spotifym3u
make install            # Build and install both to ~/.local/bin/
make install-musicsort  # Install only musicsort
make install-spotifym3u # Install only spotifym3u
make test               # Run all unit tests with coverage
make fmt                # Format all Go source files
make vet                # Run go vet for code quality checks
make lint               # Run formatters and linters
make clean              # Remove built binaries
```

## Supported Audio Formats

Both tools support the following audio formats:

- `.mp3` - MPEG Audio
- `.m4a` - MPEG-4 Audio
- `.flac` - Free Lossless Audio Codec
- `.ogg` - Ogg Vorbis
- `.opus` - Opus Audio
- `.wav` - WAV
- `.aac` - Advanced Audio Coding

## How It Works

### musicsort

1. Scans source directory for audio files
2. Reads metadata tags (artist, album, title, year)
3. Creates sanitized directory structure: `Artist/Album (Year)/`
4. Moves files to match structure
5. Removes empty directories
6. Prints status with color codes

**Example transformation:**

```
Before:
  Downloads/
  ├── song1.mp3
  ├── song2.mp3
  └── song3.flac

After:
  Music/
  ├── Artist A/
  │   └── Album 1 (2020)/
  │       ├── Artist A - Song 1.mp3
  │       └── Artist A - Song 2.mp3
  └── Artist B/
      └── Album 2 (2021)/
          └── Artist B - Song 3.flac
```

### spotifym3u

1. Parses Spotify/Exportify CSV
2. Indexes local audio files with normalized metadata
3. Matches tracks in order: exact (title+artist+album), title+artist, title-only, filename
4. Writes M3U playlist

**Match example:**

```
CSV Entry:
  Track Name: Song Title
  Artist: Artist Name
  Album: Album Name

Local File:
  Path: Music/Artist Name/Album Name/Artist Name - Song Title.mp3
  Tags: Title="Song Title", Artist="Artist Name", Album="Album Name"

Result: ✓ Matched (exact: title+artist+album)
```

## Dependencies

- [dhowden/tag](https://github.com/dhowden/tag) - Native audio metadata extraction

## Related Tools

Designed to work with:

- [Exportify](https://github.com/watsonbox/exportify) - Export Spotify playlists to CSV
- [sldl](https://github.com/fiso64/sldl) - Download from Soulseek

Workflow: Exportify → sldl → musicsort → spotifym3u

## License

MIT (or specify your license)

## Contributing

1. Fork the repo
2. Create a feature branch
3. Write tests
4. Run `make test` and `make fmt`
5. Submit a PR

## Troubleshooting

### musicsort doesn't move files

- Check directory permissions
- Use `-n` flag to preview changes
- Verify files have readable metadata tags
- Try without `-r` for top-level scan only

### spotifym3u reports missing tracks

- Ensure filenames match metadata
- Use `-debug` flag to see unmatched tracks
- Use `-recursive` flag if music is in subdirectories
- Check CSV has required columns

### Permission denied when installing

```bash
make install # installs to ~/.local/bin/
cp musicsort ~/.local/bin  # or manually
```

### Command Not Found

Ensure the files are in your `PATH`.

I have the following at the end of my `.zshrc` (or `.bashrc`)

```bash
# ~/.local/bin in path
export PATH="$HOME/.local/bin:$PATH"
```
