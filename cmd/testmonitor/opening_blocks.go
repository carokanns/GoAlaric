package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var pgnEventPattern = regexp.MustCompile(`(?m)^\[Event `)

type openingEntry struct {
	key     string
	content string
}

type openingBlockSelection struct {
	Entries     []openingEntry
	BookSHA256  string
	BlockSHA256 string
}

// prepareOpeningBlock materializes the deterministic opening subset used by
// one match block. Existing content is checked byte-for-byte so a stopped
// block can be replayed without changing its opening sample.
func prepareOpeningBlock(cfg *matchConfig) error {
	if cfg.OpeningBlockFile != "" {
		return verifyOpeningBlockFile(cfg)
	}
	if cfg.OpeningBook == "" {
		return errorsForOpeningBlock("opening book is empty")
	}
	selection, format, err := selectOpeningBlock(cfg.OpeningBook, cfg.Seed, cfg.OpeningBlockIndex, cfg.OpeningBlockSize)
	if err != nil {
		return err
	}
	name := fmt.Sprintf("openings-block-%06d.%s", cfg.OpeningBlockIndex, format)
	cfg.OpeningBlockFile = filepath.Join(cfg.RunDir, name)
	cfg.OpeningBookSHA256 = selection.BookSHA256
	cfg.OpeningBlockSHA256 = selection.BlockSHA256
	cfg.OpeningBlockColorSwap = true
	if err := writeStableFile(cfg.OpeningBlockFile, renderOpeningEntries(selection.Entries, format)); err != nil {
		return err
	}
	cfg.Openings = cfg.OpeningBlockFile
	cfg.BookFormat = format
	cfg.BookCount = len(selection.Entries)
	return nil
}

func verifyOpeningBlockFile(cfg *matchConfig) error {
	if cfg.OpeningBook == "" {
		return errorsForOpeningBlock("opening book is required to verify a materialized block")
	}
	data, err := os.ReadFile(cfg.OpeningBlockFile)
	if err != nil {
		return err
	}
	selection, format, selectErr := selectOpeningBlock(cfg.OpeningBook, cfg.Seed, cfg.OpeningBlockIndex, cfg.OpeningBlockSize)
	if selectErr != nil {
		return selectErr
	}
	expected := []byte(renderOpeningEntries(selection.Entries, format))
	if !bytes.Equal(data, expected) {
		return fmt.Errorf("materialized opening block does not match book, seed, index and size")
	}
	if cfg.OpeningBlockSHA256 != "" && digest(data) != cfg.OpeningBlockSHA256 {
		return fmt.Errorf("opening block SHA-256 changed: got %s want %s", digest(data), cfg.OpeningBlockSHA256)
	}
	cfg.Openings = cfg.OpeningBlockFile
	cfg.BookFormat = format
	cfg.BookCount = cfg.OpeningBlockSize
	cfg.OpeningBlockSHA256 = digest(data)
	cfg.OpeningBookSHA256 = selection.BookSHA256
	cfg.OpeningBlockColorSwap = true
	return nil
}

func materializeOpeningBlockCommand(args []string) error {
	fs := flag.NewFlagSet("materialize-openings", flag.ContinueOnError)
	book := fs.String("openings", "", "PGN or EPD opening book")
	fs.StringVar(book, "book", "", "alias for --openings")
	seed := fs.Int64("seed", 0, "deterministic master seed")
	blockIndex := fs.Int("block-index", 0, "zero-based opening block index")
	pairs := fs.Int("pairs", 0, "number of unique openings in the block")
	output := fs.String("output", "", "materialized PGN or EPD output path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *book == "" {
		return errorsForOpeningBlock("--openings is required")
	}
	if *pairs < 1 {
		return errorsForOpeningBlock("--pairs must be positive")
	}
	if *blockIndex < 0 {
		return errorsForOpeningBlock("--block-index must be non-negative")
	}
	if *output == "" {
		return errorsForOpeningBlock("--output is required")
	}
	bookPath, err := existingAbs(*book)
	if err != nil {
		return err
	}
	selection, format, err := selectOpeningBlock(bookPath, *seed, *blockIndex, *pairs)
	if err != nil {
		return err
	}
	outputPath, err := filepath.Abs(*output)
	if err != nil {
		return err
	}
	if err := writeStableFile(outputPath, renderOpeningEntries(selection.Entries, format)); err != nil {
		return err
	}
	report := struct {
		OpeningBook       string `json:"opening_book"`
		OpeningBookSHA256 string `json:"opening_book_sha256"`
		BlockIndex        int    `json:"opening_block_index"`
		Pairs             int    `json:"pairs_per_block"`
		Seed              int64  `json:"random_seed"`
		Format            string `json:"format"`
		Output            string `json:"output"`
		BlockSHA256       string `json:"materialized_openings_sha256"`
		ColorSwap         bool   `json:"color_swap"`
	}{
		OpeningBook:       bookPath,
		OpeningBookSHA256: selection.BookSHA256,
		BlockIndex:        *blockIndex,
		Pairs:             *pairs,
		Seed:              *seed,
		Format:            format,
		Output:            outputPath,
		BlockSHA256:       selection.BlockSHA256,
		ColorSwap:         true,
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func selectOpeningBlock(path string, seed int64, blockIndex, blockSize int) (openingBlockSelection, string, error) {
	if blockIndex < 0 || blockSize < 1 {
		return openingBlockSelection{}, "", errorsForOpeningBlock("block index and size must be non-negative and positive")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return openingBlockSelection{}, "", err
	}
	format := openingFormat(path)
	entries, err := parseOpeningEntries(string(data), format)
	if err != nil {
		return openingBlockSelection{}, "", err
	}
	if len(entries) < (blockIndex+1)*blockSize {
		return openingBlockSelection{}, "", fmt.Errorf("opening book has %d unique entries; block %d needs %d", len(entries), blockIndex, (blockIndex+1)*blockSize)
	}

	type rankedEntry struct {
		rank  [32]byte
		index int
		entry openingEntry
	}
	ranked := make([]rankedEntry, len(entries))
	for index, entry := range entries {
		ranked[index] = rankedEntry{
			rank:  sha256.Sum256([]byte(strconv.FormatInt(seed, 10) + "\n" + entry.key)),
			index: index,
			entry: entry,
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if bytes.Equal(ranked[i].rank[:], ranked[j].rank[:]) {
			return ranked[i].index < ranked[j].index
		}
		return bytes.Compare(ranked[i].rank[:], ranked[j].rank[:]) < 0
	})
	start := blockIndex * blockSize
	selected := make([]openingEntry, blockSize)
	for index := range selected {
		selected[index] = ranked[start+index].entry
	}
	rendered := renderOpeningEntries(selected, format)
	return openingBlockSelection{
		Entries: selected, BookSHA256: digest(data), BlockSHA256: digest([]byte(rendered)),
	}, format, nil
}

func parseOpeningEntries(input, format string) ([]openingEntry, error) {
	seen := make(map[string]bool)
	var entries []openingEntry
	add := func(key, content string) {
		key = strings.TrimSpace(key)
		content = strings.TrimSpace(content)
		if key == "" || content == "" || seen[key] {
			return
		}
		seen[key] = true
		entries = append(entries, openingEntry{key: key, content: content})
	}
	if format == "pgn" {
		starts := pgnEventPattern.FindAllStringIndex(input, -1)
		if len(starts) == 0 {
			return nil, errorsForOpeningBlock("PGN contains no opening events")
		}
		for index, start := range starts {
			end := len(input)
			if index+1 < len(starts) {
				end = starts[index+1][0]
			}
			content := strings.TrimSpace(input[start[0]:end])
			moves := pgnBookMovePattern.FindAllStringSubmatch(content, -1)
			var keyParts []string
			for _, match := range moves {
				keyParts = append(keyParts, match[1])
			}
			key := strings.Join(keyParts, " ")
			if key == "" {
				key = content
			}
			add(key, content)
		}
	} else {
		for _, line := range strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			add(line, line)
		}
	}
	if len(entries) == 0 {
		return nil, errorsForOpeningBlock("opening book contains no unique entries")
	}
	return entries, nil
}

func renderOpeningEntries(entries []openingEntry, format string) string {
	separator := "\n"
	if format == "pgn" {
		separator = "\n\n"
	}
	var parts []string
	for _, entry := range entries {
		parts = append(parts, entry.content)
	}
	return strings.Join(parts, separator) + "\n"
}

func writeStableFile(path, content string) error {
	data := []byte(content)
	if existing, err := os.ReadFile(path); err == nil {
		if !bytes.Equal(existing, data) {
			return fmt.Errorf("deterministic opening block already exists with different content: %s", path)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func errorsForOpeningBlock(message string) error { return fmt.Errorf("opening block: %s", message) }
