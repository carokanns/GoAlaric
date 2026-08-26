// Command siblingdiag samples normal search nodes and evaluates every legal
// sibling offline at two depths. It never changes the engine's playing code.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"goalaric/bit"
	"goalaric/board"
	"goalaric/gen"
	"goalaric/move"
	"goalaric/search"
)

const (
	schemaVersion         = 2
	defaultTraceOutput    = "/tmp/goalaric-sibling-trace-v2.json"
	defaultAnalysisOutput = "/tmp/goalaric-sibling-analysis-v2.json"
)

type sourcePosition struct {
	Index      int       `json:"index"`
	Game       int       `json:"game,omitempty"`
	GamePly    int       `json:"game_ply,omitempty"`
	BookPlies  int       `json:"book_plies,omitempty"`
	FEN        string    `json:"fen"`
	RootResult resultDoc `json:"root_result"`
	position   board.Board
}

type resultDoc struct {
	Score    int    `json:"score"`
	BestMove string `json:"best_move,omitempty"`
	Nodes    int64  `json:"nodes"`
}

type traceDocument struct {
	SchemaVersion int                           `json:"schema_version"`
	Source        string                        `json:"source"`
	RootDepth     int                           `json:"root_depth"`
	TargetDepths  []int                         `json:"target_depths"`
	PerDepth      int                           `json:"per_depth"`
	SampleSeed    uint64                        `json:"sample_seed"`
	Sources       []sourcePosition              `json:"sources"`
	Snapshots     []search.SiblingTraceSnapshot `json:"snapshots"`
	Stats         search.SiblingTraceStats      `json:"stats"`
}

type moveScore struct {
	Move  string `json:"move"`
	Score int    `json:"score"`
	Nodes int64  `json:"nodes"`
}

type depthResult struct {
	Depth       int         `json:"depth"`
	BestMove    string      `json:"best_move,omitempty"`
	BestScore   int         `json:"best_score"`
	SecondMove  string      `json:"second_move,omitempty"`
	SecondScore int         `json:"second_score"`
	GapCP       *int        `json:"gap_cp,omitempty"`
	BestIsMate  bool        `json:"best_is_mate"`
	MoveScores  []moveScore `json:"move_scores"`
}

type siblingResult struct {
	Sequence          int                          `json:"sequence"`
	SourceIndex       int                          `json:"source_index"`
	FEN               string                       `json:"fen"`
	Depth             int                          `json:"depth"`
	PieceCount        int                          `json:"piece_count"`
	LegalMoves        int                          `json:"legal_moves"`
	InCheck           bool                         `json:"in_check"`
	PVNode            bool                         `json:"pv_node"`
	ExistingExtension search.ExistingExtensionInfo `json:"existing_extension"`
	AtDepth           depthResult                  `json:"at_depth"`
	TwoPlyDeeper      depthResult                  `json:"two_ply_deeper"`
	SameBestMove      bool                         `json:"same_best_move"`
	Qualified         bool                         `json:"qualified"`
	StableSingular    bool                         `json:"stable_singular"`
	TTFound           bool                         `json:"tt_found"`
	TTDepth           int                          `json:"tt_depth,omitempty"`
	TTBound           string                       `json:"tt_bound,omitempty"`
	TTScore           int                          `json:"tt_score,omitempty"`
	TTMove            string                       `json:"tt_move,omitempty"`
	TTSignal          bool                         `json:"tt_signal"`
	TTSignalCorrect   bool                         `json:"tt_signal_correct"`
}

type interval struct {
	Low  float64 `json:"low_percent"`
	High float64 `json:"high_percent"`
}

type depthSummary struct {
	Sampled              int      `json:"sampled"`
	Qualified            int      `json:"qualified"`
	StableSingular       int      `json:"stable_singular"`
	StablePercent        float64  `json:"stable_percent"`
	Stable95CI           interval `json:"stable_95_ci"`
	TTFound              int      `json:"tt_found"`
	TTFoundPercent       float64  `json:"tt_found_percent"`
	TTSignals            int      `json:"tt_signals"`
	TTTruePositive       int      `json:"tt_true_positive"`
	TTFalsePositive      int      `json:"tt_false_positive"`
	TTRecallPercent      float64  `json:"tt_recall_percent"`
	EstimatedStablePerM  float64  `json:"estimated_stable_per_million_nodes"`
	EstimatedSignalsPerM float64  `json:"estimated_tt_signals_per_million_nodes"`
}

type gateSummary struct {
	MinimumStablePercent bool    `json:"minimum_stable_percent"`
	MinimumTTRecall      bool    `json:"minimum_tt_recall"`
	FalsePositiveLimit   bool    `json:"false_positive_limit"`
	StablePercent        float64 `json:"stable_percent"`
	TTRecallPercent      float64 `json:"tt_recall_percent"`
	FalsePositiveRatio   float64 `json:"false_positive_to_true_positive"`
	Passed               bool    `json:"passed"`
	Decision             string  `json:"decision"`
}

type analysisReport struct {
	SchemaVersion int                     `json:"schema_version"`
	Input         string                  `json:"input"`
	Summary       depthSummary            `json:"summary"`
	ByDepth       map[string]depthSummary `json:"by_depth"`
	Gate          gateSummary             `json:"gate"`
	Results       []siblingResult         `json:"results"`
}

type pgnMove struct {
	uci  string
	book bool
}
type pgnGame struct {
	fen   string
	moves []pgnMove
}

var (
	headerPattern     = regexp.MustCompile(`(?m)^\[([A-Za-z0-9_]+) "([^"]*)"\]$`)
	gameStartPattern  = regexp.MustCompile(`(?m)^\[Event `)
	moveNumberPattern = regexp.MustCompile(`^\d+\.(?:\.\.)?`)
	tokenPattern      = regexp.MustCompile(`\{[^}]*\}|[^\s]+`)
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "capture":
		err = captureCommand(os.Args[2:])
	case "analyze":
		err = analyzeCommand(os.Args[2:])
	case "help", "-h", "--help":
		usage()
		return
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "siblingdiag:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: siblingdiag capture --pgn FILE [flags]")
	fmt.Fprintln(os.Stderr, "       siblingdiag analyze --input FILE [flags]")
	fmt.Fprintln(os.Stderr, "capture uses iterative deepening and global bottom-k sampling.")
	fmt.Fprintln(os.Stderr, "analyze searches every legal sibling at depth d and d+2.")
}

func captureCommand(args []string) error {
	fs := flag.NewFlagSet("capture", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pgnPath := fs.String("pgn", "", "completed PGN in UCI notation")
	epdPath := fs.String("epd", "", "optional EPD/FEN input instead of PGN")
	rootDepth := fs.Int("depth", 10, "final iterative-deepening root depth")
	targetText := fs.String("target-depths", "6,8", "comma-separated remaining depths")
	perDepth := fs.Int("per-depth", 200, "bottom-k sample size per target depth")
	limit := fs.Int("limit", 600, "maximum source positions")
	maxGamePly := fs.Int("max-game-ply", 120, "latest eligible PGN ply")
	seed := fs.Uint64("sample-seed", 20260826, "deterministic source and node seed")
	output := fs.String("output", defaultTraceOutput, "JSON trace output")
	syzygyPath := fs.String("syzygy", "off", "Syzygy directory, or off")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if (*pgnPath == "") == (*epdPath == "") {
		return errors.New("provide exactly one of --pgn or --epd")
	}
	if *limit < 1 || *perDepth < 1 || *maxGamePly < 1 {
		return errors.New("--limit, --per-depth and --max-game-ply must be positive")
	}
	depths, err := parseDepths(*targetText)
	if err != nil {
		return err
	}
	if err := configureSyzygy(*syzygyPath); err != nil {
		return err
	}
	collector, err := search.NewSiblingTraceCollector(search.SiblingTraceConfig{Depths: depths, PerDepth: *perDepth, SampleSeed: *seed})
	if err != nil {
		return err
	}
	var sources []sourcePosition
	sourceName := *pgnPath
	if *pgnPath != "" {
		sources, err = positionsFromPGN(*pgnPath, *limit, *maxGamePly, *seed)
	} else {
		sourceName = *epdPath
		sources, err = positionsFromEPD(*epdPath, *limit)
	}
	if err != nil {
		return err
	}
	for ix := range sources {
		result, runErr := search.RunIterativeSiblingTrace(&sources[ix].position, *rootDepth, sources[ix].Index, collector)
		if runErr != nil {
			return fmt.Errorf("source %d: %w", sources[ix].Index, runErr)
		}
		sources[ix].RootResult = resultDoc{Score: result.Score, BestMove: result.BestMove, Nodes: result.Nodes}
		sources[ix].position = board.Board{}
	}
	doc := traceDocument{SchemaVersion: schemaVersion, Source: sourceName, RootDepth: *rootDepth,
		TargetDepths: depths, PerDepth: *perDepth, SampleSeed: *seed, Sources: sources,
		Snapshots: collector.Snapshots(), Stats: collector.Stats()}
	if err := writeJSON(*output, doc); err != nil {
		return err
	}
	fmt.Printf("captured sources=%d snapshots=%d", len(sources), len(doc.Snapshots))
	for _, depth := range depths {
		fmt.Printf(" depth%d=%d/%d", depth, countSnapshots(doc.Snapshots, depth), *perDepth)
	}
	fmt.Printf(" nodes=%d output=%s\n", doc.Stats.TotalSearchNodes, *output)
	return nil
}

func analyzeCommand(args []string) error {
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	input := fs.String("input", "", "JSON trace input")
	output := fs.String("output", defaultAnalysisOutput, "JSON analysis output")
	syzygyPath := fs.String("syzygy", "off", "Syzygy directory, or off")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *input == "" {
		return errors.New("--input is required")
	}
	if err := configureSyzygy(*syzygyPath); err != nil {
		return err
	}
	data, err := os.ReadFile(*input)
	if err != nil {
		return err
	}
	var trace traceDocument
	if err := json.Unmarshal(data, &trace); err != nil {
		return err
	}
	if trace.SchemaVersion != schemaVersion {
		return fmt.Errorf("unsupported trace schema %d", trace.SchemaVersion)
	}
	report := analysisReport{SchemaVersion: schemaVersion, Input: *input, ByDepth: make(map[string]depthSummary)}
	for ix, snapshot := range trace.Snapshots {
		result, analyzeErr := analyzeSnapshot(snapshot)
		if analyzeErr != nil {
			return fmt.Errorf("snapshot %d: %w", ix+1, analyzeErr)
		}
		report.Results = append(report.Results, result)
		if (ix+1)%10 == 0 || ix+1 == len(trace.Snapshots) {
			fmt.Printf("analyzed %d/%d\n", ix+1, len(trace.Snapshots))
		}
	}
	report.Summary = summarize(report.Results, 0, trace.Stats)
	for _, depth := range trace.TargetDepths {
		report.ByDepth[strconv.Itoa(depth)] = summarize(report.Results, depth, trace.Stats)
	}
	report.Gate = decideGate(report.Summary)
	if err := writeJSON(*output, report); err != nil {
		return err
	}
	fmt.Printf("stable=%d/%d (%.2f%%) tt-recall=%.2f%% gate=%s output=%s\n",
		report.Summary.StableSingular, report.Summary.Qualified, report.Summary.StablePercent,
		report.Summary.TTRecallPercent, report.Gate.Decision, *output)
	return nil
}

func analyzeSnapshot(snapshot search.SiblingTraceSnapshot) (siblingResult, error) {
	var position board.Board
	board.SetFen(snapshot.FEN, &position)
	if uint64(position.Key()) != snapshot.PositionKey {
		return siblingResult{}, errors.New("position key changed while decoding FEN")
	}
	position.SetRoot()
	legal := legalMoves(&position)
	result := siblingResult{Sequence: snapshot.Sequence, SourceIndex: snapshot.SourceIndex, FEN: snapshot.FEN,
		Depth: snapshot.Depth, PieceCount: bit.Count(position.All()), LegalMoves: len(legal), InCheck: snapshot.InCheck,
		PVNode: snapshot.PVNode, TTFound: snapshot.TTFound, TTDepth: snapshot.TTDepth, TTBound: snapshot.TTBound,
		TTScore: snapshot.TTScore, TTMove: snapshot.TTMove}
	if len(legal) < 2 {
		return result, nil
	}
	shallow, err := searchAllMoves(&position, legal, snapshot.Depth)
	if err != nil {
		return siblingResult{}, err
	}
	deep, err := searchAllMoves(&position, legal, snapshot.Depth+2)
	if err != nil {
		return siblingResult{}, err
	}
	result.AtDepth, result.TwoPlyDeeper = shallow, deep
	result.SameBestMove = shallow.BestMove == deep.BestMove
	bestMove := board.FromString(shallow.BestMove, &position)
	result.ExistingExtension = search.ClassifyExistingExtensionAtNode(&position, bestMove, snapshot.Depth, snapshot.PVNode, snapshot.RecaptureSquare)
	noMate := !shallow.BestIsMate && !deep.BestIsMate && shallow.GapCP != nil && deep.GapCP != nil
	result.Qualified = !snapshot.InCheck && len(legal) >= 4 && result.PieceCount > 4 && noMate && !result.ExistingExtension.Extended
	result.StableSingular = result.Qualified && result.SameBestMove && *shallow.GapCP >= 50 && *deep.GapCP >= 50
	result.TTSignal = result.Qualified && snapshot.TTFound && snapshot.TTMove != "" && snapshot.TTMove != "0000" &&
		(snapshot.TTBound == "lower" || snapshot.TTBound == "between") && snapshot.TTDepth >= snapshot.Depth-3 && !search.IsMateScore(snapshot.TTScore)
	result.TTSignalCorrect = result.TTSignal && result.StableSingular && snapshot.TTMove == shallow.BestMove
	return result, nil
}

func searchAllMoves(position *board.Board, legal []int, parentDepth int) (depthResult, error) {
	childDepth := parentDepth - 1
	if childDepth < 1 {
		return depthResult{}, errors.New("parent depth too shallow")
	}
	scores := make([]moveScore, 0, len(legal))
	for _, mv := range legal {
		child := *position
		child.Move(mv)
		child.SetRoot()
		searched, err := search.FixedDepthSearch(&child, childDepth)
		if err != nil {
			return depthResult{}, err
		}
		scores = append(scores, moveScore{Move: move.ToString(mv), Score: -searched.Score, Nodes: searched.Nodes})
	}
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].Score != scores[j].Score {
			return scores[i].Score > scores[j].Score
		}
		return scores[i].Move < scores[j].Move
	})
	result := depthResult{Depth: parentDepth, BestMove: scores[0].Move, BestScore: scores[0].Score,
		SecondMove: scores[1].Move, SecondScore: scores[1].Score, BestIsMate: search.IsMateScore(scores[0].Score), MoveScores: scores}
	if !search.IsMateScore(scores[0].Score) && !search.IsMateScore(scores[1].Score) {
		gap := scores[0].Score - scores[1].Score
		result.GapCP = &gap
	}
	return result, nil
}

func summarize(results []siblingResult, depth int, stats search.SiblingTraceStats) depthSummary {
	var s depthSummary
	var visited int64
	for _, result := range results {
		if depth != 0 && result.Depth != depth {
			continue
		}
		s.Sampled++
		if result.TTFound {
			s.TTFound++
		}
		if result.Qualified {
			s.Qualified++
		}
		if result.StableSingular {
			s.StableSingular++
		}
		if result.TTSignal {
			s.TTSignals++
		}
		if result.TTSignalCorrect {
			s.TTTruePositive++
		}
	}
	if depth == 0 {
		for _, count := range stats.VisitedByDepth {
			visited += count
		}
	} else {
		visited = stats.VisitedByDepth[depth]
	}
	s.TTFalsePositive = s.TTSignals - s.TTTruePositive
	s.StablePercent = percent(s.StableSingular, s.Qualified)
	s.Stable95CI = wilson95(s.StableSingular, s.Qualified)
	s.TTFoundPercent = percent(s.TTFound, s.Sampled)
	s.TTRecallPercent = percent(s.TTTruePositive, s.StableSingular)
	if s.Sampled > 0 && stats.TotalSearchNodes > 0 {
		s.EstimatedStablePerM = float64(s.StableSingular) / float64(s.Sampled) * float64(visited) / float64(stats.TotalSearchNodes) * 1e6
		s.EstimatedSignalsPerM = float64(s.TTSignals) / float64(s.Sampled) * float64(visited) / float64(stats.TotalSearchNodes) * 1e6
	}
	return s
}

func decideGate(s depthSummary) gateSummary {
	g := gateSummary{StablePercent: s.StablePercent, TTRecallPercent: s.TTRecallPercent,
		MinimumStablePercent: s.Qualified > 0 && s.StablePercent >= 3,
		MinimumTTRecall:      s.StableSingular > 0 && s.TTRecallPercent >= 70}
	if s.TTTruePositive > 0 {
		g.FalsePositiveRatio = float64(s.TTFalsePositive) / float64(s.TTTruePositive)
		g.FalsePositiveLimit = g.FalsePositiveRatio <= 2
	}
	g.Passed = g.MinimumStablePercent && g.MinimumTTRecall && g.FalsePositiveLimit
	if g.Passed {
		g.Decision = "eligible_for_experimental_extension"
	} else {
		g.Decision = "do_not_implement_extension"
	}
	return g
}

func wilson95(successes, total int) interval {
	if total == 0 {
		return interval{}
	}
	z := 1.959963984540054
	n := float64(total)
	p := float64(successes) / n
	z2 := z * z
	center := (p + z2/(2*n)) / (1 + z2/n)
	half := z * math.Sqrt(p*(1-p)/n+z2/(4*n*n)) / (1 + z2/n)
	return interval{Low: 100 * (center - half), High: 100 * (center + half)}
}

func percent(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return 100 * float64(a) / float64(b)
}

func legalMoves(position *board.Board) []int {
	var list gen.ScMvList
	gen.LegalMoves(&list, position)
	moves := make([]int, list.Size())
	for ix := range moves {
		moves[ix] = list.Move(ix)
	}
	return moves
}

func positionsFromPGN(path string, limit, maxPly int, seed uint64) ([]sourcePosition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	games, err := parsePGN(string(data))
	if err != nil {
		return nil, err
	}
	positions := make([]sourcePosition, 0, limit)
	for gameIndex, game := range games {
		if len(positions) >= limit {
			break
		}
		position, ply, bookPlies, ok, selectErr := selectPGNPosition(game, gameIndex, maxPly, seed)
		if selectErr != nil {
			return nil, fmt.Errorf("game %d: %w", gameIndex+1, selectErr)
		}
		if !ok {
			continue
		}
		position.SetRoot()
		positions = append(positions, sourcePosition{Index: len(positions), Game: gameIndex + 1, GamePly: ply,
			BookPlies: bookPlies, FEN: position.CreateFen(), position: position})
	}
	if len(positions) == 0 {
		return nil, errors.New("PGN yielded no eligible post-book positions")
	}
	return positions, nil
}

func parsePGN(input string) ([]pgnGame, error) {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	starts := gameStartPattern.FindAllStringIndex(input, -1)
	if len(starts) == 0 {
		return nil, errors.New("PGN contains no games")
	}
	results := map[string]bool{"1-0": true, "0-1": true, "1/2-1/2": true, "*": true}
	games := make([]pgnGame, 0, len(starts))
	for ix, start := range starts {
		end := len(input)
		if ix+1 < len(starts) {
			end = starts[ix+1][0]
		}
		block := input[start[0]:end]
		tags := make(map[string]string)
		for _, match := range headerPattern.FindAllStringSubmatch(block, -1) {
			tags[match[1]] = match[2]
		}
		body := headerPattern.ReplaceAllString(block, "")
		game := pgnGame{fen: tags["FEN"]}
		for _, token := range tokenPattern.FindAllString(body, -1) {
			if strings.HasPrefix(token, "{") {
				if len(game.moves) > 0 && strings.Contains(strings.ToLower(token), "book") {
					game.moves[len(game.moves)-1].book = true
				}
				continue
			}
			token = moveNumberPattern.ReplaceAllString(token, "")
			if token == "" || results[token] || strings.HasPrefix(token, "$") {
				continue
			}
			if len(token) < 4 || len(token) > 5 {
				return nil, fmt.Errorf("game %d contains unsupported move token %q", ix+1, token)
			}
			game.moves = append(game.moves, pgnMove{uci: token})
		}
		if len(game.moves) == 0 {
			return nil, fmt.Errorf("game %d has no moves", ix+1)
		}
		games = append(games, game)
	}
	return games, nil
}

func selectPGNPosition(game pgnGame, gameIndex, maxPly int, seed uint64) (board.Board, int, int, bool, error) {
	var position board.Board
	if game.fen == "" {
		board.SetFen(board.StartFen, &position)
	} else {
		board.SetFen(game.fen, &position)
	}
	bookPlies := 0
	for ix, mv := range game.moves {
		if mv.book {
			bookPlies = ix + 1
		}
	}
	type candidate struct {
		board board.Board
		ply   int
	}
	eligible := make([]candidate, 0)
	for ix, token := range game.moves {
		legal := legalMoves(&position)
		parsed := board.FromString(token.uci, &position)
		found := false
		for _, mv := range legal {
			if mv == parsed {
				found = true
				parsed = mv
				break
			}
		}
		if !found {
			return board.Board{}, 0, bookPlies, false, fmt.Errorf("illegal UCI move %q at ply %d", token.uci, ix+1)
		}
		position.Move(parsed)
		ply := ix + 1
		if ply > bookPlies && ply <= maxPly && bit.Count(position.All()) > 4 && len(legalMoves(&position)) > 0 {
			eligible = append(eligible, candidate{position, ply})
		}
	}
	if len(eligible) == 0 {
		return board.Board{}, 0, bookPlies, false, nil
	}
	rank := mix64(seed ^ uint64(gameIndex+1)*0x9e3779b97f4a7c15)
	selected := eligible[int(rank%uint64(len(eligible)))]
	return selected.board, selected.ply, bookPlies, true, nil
}

func positionsFromEPD(path string, limit int) ([]sourcePosition, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var positions []sourcePosition
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		if len(positions) >= limit {
			break
		}
		fen, ok := positionFromLine(scanner.Text())
		if !ok {
			continue
		}
		var p board.Board
		board.SetFen(fen, &p)
		p.SetRoot()
		positions = append(positions, sourcePosition{Index: len(positions), FEN: p.CreateFen(), position: p})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(positions) == 0 {
		return nil, errors.New("EPD yielded no positions")
	}
	return positions, nil
}

func positionFromLine(line string) (string, bool) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) < 4 || !strings.Contains(fields[0], "/") || (fields[1] != "w" && fields[1] != "b") {
		return "", false
	}
	return strings.Join(fields[:4], " "), true
}
func parseDepths(text string) ([]int, error) {
	var depths []int
	seen := map[int]bool{}
	for _, part := range strings.Split(text, ",") {
		d, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || d < 2 {
			return nil, fmt.Errorf("invalid target depth %q", part)
		}
		if seen[d] {
			return nil, fmt.Errorf("duplicate target depth %d", d)
		}
		seen[d] = true
		depths = append(depths, d)
	}
	sort.Ints(depths)
	return depths, nil
}
func mix64(v uint64) uint64 {
	v += 0x9e3779b97f4a7c15
	v = (v ^ (v >> 30)) * 0xbf58476d1ce4e5b9
	v = (v ^ (v >> 27)) * 0x94d049bb133111eb
	return v ^ (v >> 31)
}
func countSnapshots(items []search.SiblingTraceSnapshot, depth int) int {
	n := 0
	for _, item := range items {
		if item.Depth == depth {
			n++
		}
	}
	return n
}
func configureSyzygy(path string) error {
	path = strings.TrimSpace(path)
	if strings.EqualFold(path, "off") {
		path = ""
	}
	if _, err := search.SetSyzygyPath(path); err != nil {
		return fmt.Errorf("configure Syzygy: %w", err)
	}
	return nil
}
func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
