# musicsort

A high-performance CLI tool written in Go to organize music files based on metadata into a structured `Artist/Album (Year)/` hierarchy.

## Features
- **Concurrent Processing:** Uses Go routines to scan and move files significantly faster than shell scripts.
- **Native Metadata Parsing:** Reads tags directly from bytes without external process forking.
- **Safety First:** Includes a Dry-Run mode and "Safe Skip" logic to prevent overwriting existing files.
- **Hierarchical Organization:** Automatically creates `Artist/Album (Year)/` directory structures.

## Installation
Ensure [you have Go installed](https://go.dev/doc/install), then:

```bash
go mod init musicsort
go get [github.com/dhowden/tag](https://github.com/dhowden/tag)
go build -o musicsort
mv musicsort ~/.local/bin/
```

## Usage
```bash
musicsort [OPTIONS]
```

| Flag | Description | Default |
| :--- | :--- | :--- |
| -s | Source directory to scan | . |
| -t | Target directory for organized files | . |
| -r | Enable recursive search | false |
| -n | Dry run (preview changes without moving) | false |

## Example

### Organize a downloads folder into a music library recursively
```bash
musicsort -s ~/Downloads -t ~/Music -r
```

## Dependencies

This project utilizes the [dhowden/tag](https://github.com/dhowden/tag) library for high-performance, cross-platform audio metadata extraction. This allows the tool to parse tags natively without requiring external dependencies like ffprobe.
