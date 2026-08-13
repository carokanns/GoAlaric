package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func writeOpeningBook(t *testing.T, dir string, count int) string {
	t.Helper()
	path := filepath.Join(dir, "book.epd")
	lines := make([]string, count)
	for index := range lines {
		lines[index] = "8/8/8/8/8/8/8/8 w - - id " + strconv.Itoa(index)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOpeningBlocksAreDeterministicAndDisjoint(t *testing.T) {
	dir := t.TempDir()
	book := writeOpeningBook(t, dir, 300)

	first, firstFormat, err := selectOpeningBlock(book, 3213938493, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	second, secondFormat, err := selectOpeningBlock(book, 3213938493, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	repeat, repeatFormat, err := selectOpeningBlock(book, 3213938493, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if firstFormat != "epd" || secondFormat != firstFormat || repeatFormat != firstFormat {
		t.Fatalf("formats = %q, %q, %q", firstFormat, secondFormat, repeatFormat)
	}
	if first.BlockSHA256 != repeat.BlockSHA256 || first.BookSHA256 != repeat.BookSHA256 || !bytes.Equal([]byte(renderOpeningEntries(first.Entries, firstFormat)), []byte(renderOpeningEntries(repeat.Entries, repeatFormat))) {
		t.Fatal("same book, seed and block index produced different opening block")
	}
	seen := make(map[string]bool)
	for _, entry := range first.Entries {
		seen[entry.key] = true
	}
	for _, entry := range second.Entries {
		if seen[entry.key] {
			t.Fatalf("opening %q appears in two blocks", entry.key)
		}
	}
}

func TestMaterializeOpeningBlockCommandIsStable(t *testing.T) {
	dir := t.TempDir()
	book := writeOpeningBook(t, dir, 120)
	output := filepath.Join(dir, "materialized.epd")
	args := []string{"--openings", book, "--seed", "77", "--block-index", "2", "--pairs", "10", "--output", output}
	if err := materializeOpeningBlockCommand(args); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := materializeOpeningBlockCommand(args); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("materialized opening command changed an existing block")
	}
}

func TestStoppedOpeningBlockCanBeReplayedByteForByte(t *testing.T) {
	dir := t.TempDir()
	book := writeOpeningBook(t, dir, 120)
	first := matchConfig{
		Openings: book, OpeningBook: book, OpeningBlockIndex: 1, OpeningBlockSize: 20,
		Seed: 987654321, RunDir: filepath.Join(dir, "block"),
	}
	if err := os.MkdirAll(first.RunDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := prepareOpeningBlock(&first); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(first.OpeningBlockFile)
	if err != nil {
		t.Fatal(err)
	}

	second := matchConfig{
		Openings: book, OpeningBook: book, OpeningBlockIndex: 1, OpeningBlockSize: 20,
		Seed: 987654321, RunDir: first.RunDir,
	}
	if err := prepareOpeningBlock(&second); err != nil {
		t.Fatal(err)
	}
	replayed, err := os.ReadFile(second.OpeningBlockFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, replayed) || first.OpeningBlockSHA256 != second.OpeningBlockSHA256 {
		t.Fatal("replayed stopped block differs from the original block")
	}

	if err := os.WriteFile(first.OpeningBlockFile, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	third := matchConfig{
		Openings: book, OpeningBook: book, OpeningBlockIndex: 1, OpeningBlockSize: 20,
		Seed: 987654321, RunDir: first.RunDir,
	}
	if err := prepareOpeningBlock(&third); err == nil {
		t.Fatal("changed existing block was accepted")
	}
}

func TestCompletedOpeningBlockIsCountedOnlyOnce(t *testing.T) {
	dir := t.TempDir()
	book := writeOpeningBook(t, dir, 100)
	fastchess := filepath.Join(dir, "fastchess")
	if err := os.WriteFile(fastchess, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	baseline := filepath.Join(dir, "baseline")
	candidate := filepath.Join(dir, "candidate")
	if err := os.WriteFile(baseline, []byte("baseline"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(candidate, []byte("candidate"), 0o755); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(dir, "completed")
	if err := saveStatus(runDir, &matchStatus{State: "completed", Games: 100, RunDir: runDir}); err != nil {
		t.Fatal(err)
	}
	if err := startCommand([]string{
		"--fastchess", fastchess, "--baseline", baseline, "--candidate", candidate,
		"--openings", book, "--games", "100", "--run-dir", runDir, "--syzygy-path", "off",
	}); err != nil {
		t.Fatal(err)
	}
	status, err := loadStatus(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "completed" || status.Games != 100 {
		t.Fatalf("completed block was changed on second start: %+v", status)
	}
}

func TestFakeFastchessUsesMaterializedBlockAndColorSwappedPairs(t *testing.T) {
	dir := t.TempDir()
	book := writeOpeningBook(t, dir, 100)
	blockCfg := matchConfig{
		Openings: book, OpeningBook: book, OpeningBlockIndex: 0, OpeningBlockSize: 1,
		Seed: 12345, RunDir: filepath.Join(dir, "run"), Games: 2,
	}
	if err := os.MkdirAll(blockCfg.RunDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := prepareOpeningBlock(&blockCfg); err != nil {
		t.Fatal(err)
	}

	fastchess := filepath.Join(dir, "fastchess")
	argsFile := filepath.Join(dir, "fastchess.args")
	if err := os.WriteFile(fastchess, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$FAKE_FASTCHESS_ARGS\"\npgn=\"\"\nwhile [ \"$#\" -gt 0 ]; do if [ \"$1\" = \"-pgnout\" ]; then shift; pgn=$(printf '%s' \"$1\" | sed 's/^file=//'); break; fi; shift; done\ncat > \"$pgn\" <<'EOF'\n[Event \"match\"]\n[Round \"1.1\"]\n[FEN \"8/8/8/8/8/8/8/8 w - - 0 1\"]\n\n1. e2e4 1/2-1/2\n\n[Event \"match\"]\n[Round \"1.2\"]\n[FEN \"8/8/8/8/8/8/8/8 w - - 0 1\"]\n\n1. e2e4 1/2-1/2\n\nEOF\necho 'Score of Candidate vs Baseline: 0 - 0 - 2  [0.500] 2'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_FASTCHESS_ARGS", argsFile)
	baseline := filepath.Join(dir, "baseline")
	candidate := filepath.Join(dir, "candidate")
	if err := os.WriteFile(baseline, []byte("baseline"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(candidate, []byte("candidate"), 0o755); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(dir, "match")
	if err := runMatchCommand([]string{
		"--fastchess", fastchess, "--baseline", baseline, "--candidate", candidate,
		"--openings", book, "--opening-block-file", blockCfg.OpeningBlockFile,
		"--block-index", "0", "--block-size", "1", "--seed", "12345", "--games", "2", "--concurrency", "1",
		"--tc", "10+0.1", "--syzygy-path", "off", "--run-dir", runDir,
	}); err != nil {
		t.Fatal(err)
	}
	status, err := loadStatus(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if !status.OpeningBlockColorSwap || status.OpeningBlockSHA256 == "" || status.OpeningBookSHA256 == "" {
		t.Fatalf("block metadata missing: %+v", status)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "-repeat") || !strings.Contains(string(args), "-rounds") {
		t.Fatalf("Fastchess was not configured for paired color-swapped openings: %s", args)
	}
}

func TestInterruptedBlockIsInvalidAndCanBeReplayedWithFakeFastchess(t *testing.T) {
	dir := t.TempDir()
	book := writeOpeningBook(t, dir, 120)
	fastchess := filepath.Join(dir, "fastchess")
	stateFile := filepath.Join(dir, "fastchess-first-run")
	script := `#!/bin/sh
pgn=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-pgnout" ]; then shift; pgn=$(printf '%s' "$1" | sed 's/^file=//'); break; fi
  shift
done
if [ ! -f "$FAKE_FASTCHESS_STATE" ]; then
  touch "$FAKE_FASTCHESS_STATE"
  cat > "$pgn" <<'EOF'
[Event "match"]
[Round "4.1"]
[FEN "8/8/8/8/8/8/8/8 w - - 0 1"]

1. e2e4 1/2-1/2

EOF
  echo 'Score of Candidate vs Baseline: 0 - 0 - 1  [0.500] 1'
  exit 7
fi
cat > "$pgn" <<'EOF'
[Event "match"]
[Round "4.1"]
[FEN "8/8/8/8/8/8/8/8 w - - 0 1"]

1. e2e4 1/2-1/2

[Event "match"]
[Round "4.2"]
[FEN "8/8/8/8/8/8/8/8 w - - 0 1"]

1. e2e4 1/2-1/2

EOF
echo 'Score of Candidate vs Baseline: 0 - 0 - 2  [0.500] 2'
`
	if err := os.WriteFile(fastchess, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_FASTCHESS_STATE", stateFile)
	baseline := filepath.Join(dir, "baseline")
	candidate := filepath.Join(dir, "candidate")
	if err := os.WriteFile(baseline, []byte("baseline"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(candidate, []byte("candidate"), 0o755); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(dir, "block-3")
	args := []string{
		"--fastchess", fastchess, "--baseline", baseline, "--candidate", candidate,
		"--openings", book, "--block-index", "3", "--block-size", "1", "--seed", "4242",
		"--games", "2", "--concurrency", "1", "--tc", "10+0.1", "--syzygy-path", "off", "--run-dir", runDir,
	}
	if err := runMatchCommand(args); err == nil {
		t.Fatal("interrupted fake Fastchess run unexpectedly succeeded")
	}
	firstStatus, err := loadStatus(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if firstStatus.State != "failed" || firstStatus.Decision != "" {
		t.Fatalf("interrupted block was not rejected as a failed run: %+v", firstStatus)
	}
	firstReport, err := loadOpeningBlockReport(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if firstReport.State != "invalid" || firstReport.Valid || firstReport.Counted {
		t.Fatalf("interrupted block was counted: %+v", firstReport)
	}
	blockPath := filepath.Join(runDir, "openings-block-000003.epd")
	firstBlock, err := os.ReadFile(blockPath)
	if err != nil {
		t.Fatal(err)
	}

	if err := runMatchCommand(args); err != nil {
		t.Fatal(err)
	}
	secondStatus, err := loadStatus(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if secondStatus.State != "completed" || secondStatus.Games != 2 {
		t.Fatalf("replayed block did not complete: %+v", secondStatus)
	}
	secondReport, err := loadOpeningBlockReport(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if secondReport.State != "completed" || !secondReport.Valid || !secondReport.Counted || secondReport.Games != 2 {
		t.Fatalf("replayed block was not counted exactly once: %+v", secondReport)
	}
	secondBlock, err := os.ReadFile(blockPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBlock, secondBlock) || firstReport.OpeningBlockSHA256 != secondReport.OpeningBlockSHA256 {
		t.Fatal("replayed block did not use the identical materialized openings")
	}

	if err := startCommand(args); err != nil {
		t.Fatal(err)
	}
	thirdReport, err := loadOpeningBlockReport(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if !thirdReport.Counted || thirdReport.State != "completed" {
		t.Fatalf("completed block was not preserved on restart: %+v", thirdReport)
	}
}
