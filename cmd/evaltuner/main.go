// Command evaltuner creates leakage-resistant position datasets and tunes a
// small, explicit family of GoAlaric evaluation parameters.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
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

	"goalaric/board"
	"goalaric/eval"
)

type datasetRecord struct {
	SchemaVersion int     `json:"schema_version"`
	GameID        string  `json:"game_id"`
	GroupID       string  `json:"group_id"`
	Source        string  `json:"source"`
	Ply           int     `json:"ply"`
	FEN           string  `json:"fen"`
	Result        float64 `json:"result"`
}

type datasetReport struct {
	SchemaVersion       int      `json:"schema_version"`
	Seed                uint64   `json:"seed"`
	ValidationPercent   int      `json:"validation_percent"`
	GroupPlies          int      `json:"group_plies"`
	MinimumPly          int      `json:"minimum_ply"`
	PlyStride           int      `json:"ply_stride"`
	MaximumPerGame      int      `json:"maximum_positions_per_game"`
	Sources             []string `json:"sources"`
	SourceSHA256        []string `json:"source_sha256"`
	Games               int      `json:"games"`
	SkippedGames        int      `json:"skipped_games"`
	DuplicatePositions  int      `json:"duplicate_positions"`
	TrainingGames       int      `json:"training_games"`
	ValidationGames     int      `json:"validation_games"`
	TrainingGroups      int      `json:"training_groups"`
	ValidationGroups    int      `json:"validation_groups"`
	TrainingPositions   int      `json:"training_positions"`
	ValidationPositions int      `json:"validation_positions"`
	TrainingPath        string   `json:"training_path"`
	ValidationPath      string   `json:"validation_path"`
	TrainingSHA256      string   `json:"training_sha256"`
	ValidationSHA256    string   `json:"validation_sha256"`
}

type pgnGame struct {
	Tags   map[string]string
	Moves  []string
	Result string
}

type tuneReport struct {
	SchemaVersion    int                       `json:"schema_version"`
	TrainingPath     string                    `json:"training_path"`
	ValidationPath   string                    `json:"validation_path"`
	TrainingSHA256   string                    `json:"training_sha256"`
	ValidationSHA256 string                    `json:"validation_sha256"`
	TrainingCount    int                       `json:"training_count"`
	ValidationCount  int                       `json:"validation_count"`
	K                float64                   `json:"k"`
	Initial          eval.PawnStructureWeights `json:"initial"`
	Tuned            eval.PawnStructureWeights `json:"tuned"`
	InitialTrain     float64                   `json:"initial_training_error"`
	TunedTrain       float64                   `json:"tuned_training_error"`
	InitialValidate  float64                   `json:"initial_validation_error"`
	TunedValidate    float64                   `json:"tuned_validation_error"`
	Steps            []int                     `json:"steps"`
	Passes           int                       `json:"passes"`
}

var (
	tagPattern  = regexp.MustCompile(`^\[([A-Za-z0-9_]+)\s+"(.*)"\]$`)
	movePattern = regexp.MustCompile(`^[a-h][1-8][a-h][1-8][qrbn]?$`)
)

func main() {
	if len(os.Args) < 2 {
		fatal(errors.New("usage: evaltuner <dataset|tune> [options]"))
	}
	var err error
	switch os.Args[1] {
	case "dataset":
		err = datasetCommand(os.Args[2:])
	case "tune":
		err = tuneCommand(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "evaltuner:", err)
	os.Exit(1)
}

func datasetCommand(args []string) error {
	fs := flag.NewFlagSet("dataset", flag.ContinueOnError)
	var inputs stringList
	fs.Var(&inputs, "pgn", "input PGN (repeatable)")
	outputDir := fs.String("output-dir", "artifacts/evaltuner/dataset", "dataset directory")
	seed := fs.Uint64("seed", 42, "deterministic split seed")
	validationPercent := fs.Int("validation-percent", 20, "validation groups in percent")
	groupPlies := fs.Int("group-plies", 16, "opening plies used to keep paired games together")
	minimumPly := fs.Int("minimum-ply", 20, "first sampled ply")
	stride := fs.Int("stride", 8, "sample every N plies")
	maxPerGame := fs.Int("max-per-game", 12, "maximum sampled positions per game")
	maxGames := fs.Int("max-games", 0, "maximum games; zero means all")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(inputs) == 0 {
		return errors.New("at least one --pgn is required")
	}
	if *validationPercent < 1 || *validationPercent > 99 || *groupPlies < 1 || *minimumPly < 1 || *stride < 1 || *maxPerGame < 1 || *maxGames < 0 {
		return errors.New("invalid dataset limits")
	}

	outAbs, err := filepath.Abs(*outputDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outAbs, 0o755); err != nil {
		return err
	}
	trainPath := filepath.Join(outAbs, "train.jsonl")
	validationPath := filepath.Join(outAbs, "validation.jsonl")
	trainFile, err := os.Create(trainPath)
	if err != nil {
		return err
	}
	defer trainFile.Close()
	validationFile, err := os.Create(validationPath)
	if err != nil {
		return err
	}
	defer validationFile.Close()
	trainWriter := bufio.NewWriter(trainFile)
	defer trainWriter.Flush()
	validationWriter := bufio.NewWriter(validationFile)
	defer validationWriter.Flush()

	report := datasetReport{SchemaVersion: 1, Seed: *seed, ValidationPercent: *validationPercent, GroupPlies: *groupPlies, MinimumPly: *minimumPly, PlyStride: *stride, MaximumPerGame: *maxPerGame, TrainingPath: trainPath, ValidationPath: validationPath}
	seenFEN := make(map[string]struct{})
	trainGames := make(map[string]struct{})
	validationGames := make(map[string]struct{})
	trainGroups := make(map[string]struct{})
	validationGroups := make(map[string]struct{})
	stop := false
	for _, input := range inputs {
		inputAbs, err := filepath.Abs(input)
		if err != nil {
			return err
		}
		report.Sources = append(report.Sources, inputAbs)
		inputSHA, err := fileSHA256(inputAbs)
		if err != nil {
			return err
		}
		report.SourceSHA256 = append(report.SourceSHA256, inputSHA)
		err = scanPGN(inputAbs, func(game pgnGame) error {
			if stop {
				return nil
			}
			if *maxGames > 0 && report.Games >= *maxGames {
				stop = true
				return nil
			}
			records, validation, gameID, err := recordsForGame(inputAbs, game, *seed, *validationPercent, *groupPlies, *minimumPly, *stride, *maxPerGame)
			if err != nil || len(records) == 0 {
				report.SkippedGames++
				return nil
			}
			report.Games++
			writer := trainWriter
			if validation {
				writer = validationWriter
				validationGames[gameID] = struct{}{}
				validationGroups[records[0].GroupID] = struct{}{}
			} else {
				trainGames[gameID] = struct{}{}
				trainGroups[records[0].GroupID] = struct{}{}
			}
			for _, record := range records {
				if _, duplicate := seenFEN[record.FEN]; duplicate {
					report.DuplicatePositions++
					continue
				}
				seenFEN[record.FEN] = struct{}{}
				data, _ := json.Marshal(record)
				if _, err := writer.Write(append(data, '\n')); err != nil {
					return err
				}
				if validation {
					report.ValidationPositions++
				} else {
					report.TrainingPositions++
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		if stop {
			break
		}
	}
	report.TrainingGames = len(trainGames)
	report.ValidationGames = len(validationGames)
	report.TrainingGroups = len(trainGroups)
	report.ValidationGroups = len(validationGroups)
	for groupID := range trainGroups {
		if _, leaked := validationGroups[groupID]; leaked {
			return fmt.Errorf("opening group %s occurs in both dataset partitions", groupID)
		}
	}
	if err := trainWriter.Flush(); err != nil {
		return err
	}
	if err := validationWriter.Flush(); err != nil {
		return err
	}
	if report.TrainingPositions == 0 || report.ValidationPositions == 0 {
		return errors.New("dataset split produced an empty partition")
	}
	report.TrainingSHA256, err = fileSHA256(trainPath)
	if err != nil {
		return err
	}
	report.ValidationSHA256, err = fileSHA256(validationPath)
	if err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(outAbs, "manifest.json"), report); err != nil {
		return err
	}
	fmt.Printf("dataset saved: %s\ntrain=%d validation=%d games=%d skipped=%d duplicates=%d\n", outAbs, report.TrainingPositions, report.ValidationPositions, report.Games, report.SkippedGames, report.DuplicatePositions)
	return nil
}

func recordsForGame(source string, game pgnGame, seed uint64, validationPercent, groupPlies, minimumPly, stride, maxPerGame int) ([]datasetRecord, bool, string, error) {
	result, ok := resultValue(game.Result)
	if !ok || len(game.Moves) < minimumPly {
		return nil, false, "", errors.New("game has no usable result or moves")
	}
	startFEN := board.StartFen
	if fen := game.Tags["FEN"]; fen != "" {
		startFEN = fen
	}
	openingCount := groupPlies
	if openingCount > len(game.Moves) {
		openingCount = len(game.Moves)
	}
	groupID := hashString(fmt.Sprintf("%d|%s|%s", seed, startFEN, strings.Join(game.Moves[:openingCount], " ")))
	gameID := hashString(source + "|" + game.Tags["Round"] + "|" + game.Tags["White"] + "|" + game.Tags["Black"] + "|" + strings.Join(game.Moves, " "))
	validation := splitValue(seed, groupID) < uint64(validationPercent)

	var bd board.Board
	board.SetFen(startFEN, &bd)
	records := make([]datasetRecord, 0, maxPerGame)
	for ix, moveText := range game.Moves {
		if len(moveText) < 4 {
			return nil, false, "", fmt.Errorf("invalid move %q", moveText)
		}
		mv := board.FromString(moveText, &bd)
		if !board.IsMove(mv, &bd) {
			return nil, false, "", fmt.Errorf("illegal move %q at ply %d", moveText, ix+1)
		}
		bd.MakeFenMve(mv)
		ply := ix + 1
		if ply >= minimumPly && (ply-minimumPly)%stride == 0 && len(records) < maxPerGame {
			records = append(records, datasetRecord{SchemaVersion: 1, GameID: gameID, GroupID: groupID, Source: source, Ply: ply, FEN: bd.CreateFen(), Result: result})
		}
	}
	return records, validation, gameID, nil
}

func scanPGN(path string, consume func(pgnGame) error) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	game := pgnGame{Tags: make(map[string]string)}
	var movetext strings.Builder
	flush := func() error {
		if movetext.Len() == 0 {
			return nil
		}
		game.Moves = coordinateMoves(movetext.String())
		game.Result = game.Tags["Result"]
		if err := consume(game); err != nil {
			return err
		}
		game = pgnGame{Tags: make(map[string]string)}
		movetext.Reset()
		return nil
	}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if match := tagPattern.FindStringSubmatch(line); match != nil {
			if match[1] == "Event" && movetext.Len() != 0 {
				if err := flush(); err != nil {
					return err
				}
			}
			game.Tags[match[1]] = match[2]
			continue
		}
		if line != "" {
			movetext.WriteByte(' ')
			movetext.WriteString(line)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return flush()
}

func coordinateMoves(text string) []string {
	var clean strings.Builder
	braceDepth, variationDepth := 0, 0
	for _, ch := range text {
		switch ch {
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case '(':
			if braceDepth == 0 {
				variationDepth++
			}
		case ')':
			if braceDepth == 0 && variationDepth > 0 {
				variationDepth--
			}
		default:
			if braceDepth == 0 && variationDepth == 0 {
				clean.WriteRune(ch)
			}
		}
	}
	var moves []string
	for _, token := range strings.Fields(clean.String()) {
		token = strings.TrimSpace(token)
		if movePattern.MatchString(token) {
			moves = append(moves, token)
		}
	}
	return moves
}

func resultValue(result string) (float64, bool) {
	switch result {
	case "1-0":
		return 1, true
	case "0-1":
		return 0, true
	case "1/2-1/2":
		return 0.5, true
	default:
		return 0, false
	}
}

func splitValue(seed uint64, groupID string) uint64 {
	sum := sha256.Sum256([]byte(strconv.FormatUint(seed, 10) + "|" + groupID))
	return binary.BigEndian.Uint64(sum[:8]) % 100
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:12])
}

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func tuneCommand(args []string) error {
	fs := flag.NewFlagSet("tune", flag.ContinueOnError)
	trainPath := fs.String("train", "", "training JSONL")
	validationPath := fs.String("validation", "", "validation JSONL")
	output := fs.String("output", "artifacts/evaltuner/tune-result.json", "result JSON")
	stepsText := fs.String("steps", "4,2,1", "coordinate steps")
	passes := fs.Int("passes", 2, "maximum passes per step")
	maxTrain := fs.Int("max-train", 0, "maximum training positions")
	maxValidation := fs.Int("max-validation", 0, "maximum validation positions")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *trainPath == "" || *validationPath == "" || *passes < 1 {
		return errors.New("--train, --validation and positive --passes are required")
	}
	steps, err := parseSteps(*stepsText)
	if err != nil {
		return err
	}
	train, err := readDataset(*trainPath, *maxTrain)
	if err != nil {
		return err
	}
	validation, err := readDataset(*validationPath, *maxValidation)
	if err != nil {
		return err
	}
	if len(train) == 0 || len(validation) == 0 {
		return errors.New("empty training or validation data")
	}
	initial := eval.CurrentPawnStructureWeights()
	defer eval.SetPawnStructureWeights(initial)
	trainSHA, err := fileSHA256(*trainPath)
	if err != nil {
		return err
	}
	validationSHA, err := fileSHA256(*validationPath)
	if err != nil {
		return err
	}
	initialScores := evaluateScores(train, initial)
	k := calibrateK(train, initialScores)
	initialTrain := predictionError(train, initialScores, k)
	initialValidation := loss(validation, initial, k)
	current := initial
	currentLoss := initialTrain
	for _, step := range steps {
		for pass := 0; pass < *passes; pass++ {
			improved := false
			for parameter := 0; parameter < 6; parameter++ {
				bestWeights, bestLoss := current, currentLoss
				for _, delta := range []int{-step, step} {
					candidate := changeWeight(current, parameter, delta)
					if weightValue(candidate, parameter) < 0 || weightValue(candidate, parameter) > 80 {
						continue
					}
					candidateLoss := loss(train, candidate, k)
					if candidateLoss+1e-12 < bestLoss {
						bestWeights, bestLoss = candidate, candidateLoss
					}
				}
				if bestWeights != current {
					current, currentLoss, improved = bestWeights, bestLoss, true
				}
			}
			fmt.Printf("step=%d pass=%d train_error=%.9f weights=%+v\n", step, pass+1, currentLoss, current)
			if !improved {
				break
			}
		}
	}
	report := tuneReport{SchemaVersion: 1, TrainingPath: *trainPath, ValidationPath: *validationPath, TrainingSHA256: trainSHA, ValidationSHA256: validationSHA, TrainingCount: len(train), ValidationCount: len(validation), K: k, Initial: initial, Tuned: current, InitialTrain: initialTrain, TunedTrain: currentLoss, InitialValidate: initialValidation, TunedValidate: loss(validation, current, k), Steps: steps, Passes: *passes}
	if err := writeJSON(*output, report); err != nil {
		return err
	}
	fmt.Printf("tuning saved: %s\ntrain %.9f -> %.9f validation %.9f -> %.9f\n", *output, report.InitialTrain, report.TunedTrain, report.InitialValidate, report.TunedValidate)
	return nil
}

func readDataset(path string, limit int) ([]datasetRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var records []datasetRecord
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record datasetRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, err
		}
		records = append(records, record)
		if limit > 0 && len(records) >= limit {
			break
		}
	}
	return records, scanner.Err()
}

func loss(records []datasetRecord, weights eval.PawnStructureWeights, k float64) float64 {
	return predictionError(records, evaluateScores(records, weights), k)
}

func evaluateScores(records []datasetRecord, weights eval.PawnStructureWeights) []int {
	eval.SetPawnStructureWeights(weights)
	var pawnHash eval.PawnHash
	pawnHash.Clear()
	scores := make([]int, len(records))
	var bd board.Board
	for ix, record := range records {
		board.SetFen(record.FEN, &bd)
		scores[ix] = eval.CompEval(&bd, &pawnHash)
	}
	return scores
}

func predictionError(records []datasetRecord, scores []int, k float64) float64 {
	errorSum := 0.0
	for ix, score := range scores {
		prediction := 1.0 / (1.0 + math.Pow(10, -k*float64(score)/400.0))
		delta := records[ix].Result - prediction
		errorSum += delta * delta
	}
	return errorSum / float64(len(records))
}

func calibrateK(records []datasetRecord, scores []int) float64 {
	bestK, bestLoss := 0.50, math.MaxFloat64
	for step := 50; step <= 200; step++ {
		k := float64(step) / 100.0
		candidate := predictionError(records, scores, k)
		if candidate < bestLoss {
			bestK, bestLoss = k, candidate
		}
	}
	return bestK
}

func changeWeight(weights eval.PawnStructureWeights, parameter, delta int) eval.PawnStructureWeights {
	switch parameter {
	case 0:
		weights.IsolatedMG += delta
	case 1:
		weights.IsolatedEG += delta
	case 2:
		weights.WeakMG += delta
	case 3:
		weights.WeakEG += delta
	case 4:
		weights.DoubledMG += delta
	case 5:
		weights.DoubledEG += delta
	}
	return weights
}

func weightValue(weights eval.PawnStructureWeights, parameter int) int {
	return []int{weights.IsolatedMG, weights.IsolatedEG, weights.WeakMG, weights.WeakEG, weights.DoubledMG, weights.DoubledEG}[parameter]
}

func parseSteps(value string) ([]int, error) {
	var steps []int
	for _, field := range strings.Split(value, ",") {
		step, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil || step < 1 {
			return nil, fmt.Errorf("invalid step %q", field)
		}
		steps = append(steps, step)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(steps)))
	return steps, nil
}

func writeJSON(path string, value any) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(abs, append(data, '\n'), 0o644)
}

type stringList []string

func (values *stringList) String() string { return strings.Join(*values, ",") }
func (values *stringList) Set(value string) error {
	*values = append(*values, value)
	return nil
}
