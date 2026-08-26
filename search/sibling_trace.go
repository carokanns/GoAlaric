package search

import (
	"container/heap"
	"errors"
	"fmt"
	"sort"

	"goalaric/board"
	"goalaric/hash"
	"goalaric/move"
)

// SiblingTraceConfig controls deterministic bottom-k sampling. Depths are
// remaining search depths, not root iteration depths.
type SiblingTraceConfig struct {
	Depths     []int
	PerDepth   int
	SampleSeed uint64
}

// SiblingTraceSnapshot is a replayable search node selected from the final
// iteration after normal iterative-deepening warm-up.
type SiblingTraceSnapshot struct {
	SchemaVersion   int    `json:"schema_version"`
	Sequence        int    `json:"sequence"`
	SourceIndex     int    `json:"source_index"`
	SampleRank      uint64 `json:"sample_rank"`
	FEN             string `json:"fen"`
	PositionKey     uint64 `json:"position_key"`
	Depth           int    `json:"depth"`
	Ply             int    `json:"ply"`
	Alpha           int    `json:"alpha"`
	Beta            int    `json:"beta"`
	PVNode          bool   `json:"pv_node"`
	InCheck         bool   `json:"in_check"`
	HardPruning     bool   `json:"hard_pruning"`
	RecaptureSquare int    `json:"recapture_square"`
	TTFound         bool   `json:"tt_found"`
	TTDepthValid    bool   `json:"tt_depth_valid"`
	TTDepth         int    `json:"tt_depth,omitempty"`
	TTMove          string `json:"tt_move,omitempty"`
	TTScore         int    `json:"tt_score,omitempty"`
	TTBound         string `json:"tt_bound,omitempty"`
	SearchNodes     int64  `json:"search_nodes"`
}

type FixedDepthResult struct {
	Score    int
	BestMove string
	Nodes    int64
}

type SiblingTraceStats struct {
	VisitedByDepth   map[int]int64 `json:"visited_by_depth"`
	TotalSearchNodes int64         `json:"total_search_nodes"`
}

type sampledSnapshot struct {
	rank uint64
	item SiblingTraceSnapshot
}

type maxSnapshotHeap []sampledSnapshot

func (h maxSnapshotHeap) Len() int { return len(h) }
func (h maxSnapshotHeap) Less(i, j int) bool {
	if h[i].rank != h[j].rank {
		return h[i].rank > h[j].rank
	}
	return snapshotIdentity(h[i].item) > snapshotIdentity(h[j].item)
}
func (h maxSnapshotHeap) Swap(i, j int)   { h[i], h[j] = h[j], h[i] }
func (h *maxSnapshotHeap) Push(value any) { *h = append(*h, value.(sampledSnapshot)) }
func (h *maxSnapshotHeap) Pop() any {
	old := *h
	value := old[len(old)-1]
	*h = old[:len(old)-1]
	return value
}

// SiblingTraceCollector spans all source positions so the sample is global,
// stratified and independent of traversal order.
type SiblingTraceCollector struct {
	config        SiblingTraceConfig
	target        map[int]bool
	heaps         map[int]*maxSnapshotHeap
	seen          map[string]struct{}
	visited       map[int]int64
	totalNodes    int64
	currentSource int
}

var siblingTrace *SiblingTraceCollector

func NewSiblingTraceCollector(config SiblingTraceConfig) (*SiblingTraceCollector, error) {
	if config.PerDepth < 1 {
		return nil, errors.New("sibling trace per-depth quota must be positive")
	}
	if len(config.Depths) == 0 {
		return nil, errors.New("sibling trace requires at least one target depth")
	}
	c := &SiblingTraceCollector{
		config: config, target: make(map[int]bool), heaps: make(map[int]*maxSnapshotHeap),
		seen: make(map[string]struct{}), visited: make(map[int]int64),
	}
	for _, depth := range config.Depths {
		if depth < 2 || depth > maxDepth {
			return nil, fmt.Errorf("trace depth must be between 2 and %d", maxDepth)
		}
		if c.target[depth] {
			return nil, fmt.Errorf("duplicate trace depth %d", depth)
		}
		c.target[depth] = true
		h := maxSnapshotHeap{}
		heap.Init(&h)
		c.heaps[depth] = &h
	}
	return c, nil
}

func (c *SiblingTraceCollector) Snapshots() []SiblingTraceSnapshot {
	items := make([]SiblingTraceSnapshot, 0, len(c.heaps)*c.config.PerDepth)
	for _, h := range c.heaps {
		for _, sampled := range *h {
			items = append(items, sampled.item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Depth != items[j].Depth {
			return items[i].Depth < items[j].Depth
		}
		if items[i].SampleRank != items[j].SampleRank {
			return items[i].SampleRank < items[j].SampleRank
		}
		return snapshotIdentity(items[i]) < snapshotIdentity(items[j])
	})
	for i := range items {
		items[i].Sequence = i + 1
	}
	return items
}

func (c *SiblingTraceCollector) Stats() SiblingTraceStats {
	visited := make(map[int]int64, len(c.visited))
	for depth, count := range c.visited {
		visited[depth] = count
	}
	return SiblingTraceStats{VisitedByDepth: visited, TotalSearchNodes: c.totalNodes}
}

func snapshotIdentity(s SiblingTraceSnapshot) string {
	return fmt.Sprintf("%016x/%d/%d", s.PositionKey, s.Depth, s.SourceIndex)
}

func splitmix64(value uint64) uint64 {
	value += 0x9e3779b97f4a7c15
	value = (value ^ (value >> 30)) * 0xbf58476d1ce4e5b9
	value = (value ^ (value >> 27)) * 0x94d049bb133111eb
	return value ^ (value >> 31)
}

func (c *SiblingTraceCollector) consider(snapshot SiblingTraceSnapshot) {
	c.visited[snapshot.Depth]++
	identity := snapshotIdentity(snapshot)
	if _, exists := c.seen[identity]; exists {
		return
	}
	c.seen[identity] = struct{}{}
	snapshot.SampleRank = splitmix64(snapshot.PositionKey ^ uint64(snapshot.Depth)<<48 ^ uint64(snapshot.SourceIndex) ^ c.config.SampleSeed)
	h := c.heaps[snapshot.Depth]
	sampled := sampledSnapshot{rank: snapshot.SampleRank, item: snapshot}
	if h.Len() < c.config.PerDepth {
		heap.Push(h, sampled)
		return
	}
	worst := (*h)[0]
	if sampled.rank < worst.rank || (sampled.rank == worst.rank && snapshotIdentity(snapshot) < snapshotIdentity(worst.item)) {
		heap.Pop(h)
		heap.Push(h, sampled)
	}
}

func FixedDepthSearch(position *board.Board, depth int) (FixedDepthResult, error) {
	if position == nil {
		return FixedDepthResult{}, errors.New("position is nil")
	}
	if depth < 1 || depth > maxDepth {
		return FixedDepthResult{}, fmt.Errorf("depth must be between 1 and %d", maxDepth)
	}
	if SearchStatus() == Running {
		return FixedDepthResult{}, errors.New("cannot run sibling diagnostic during an active search")
	}
	return runFixedDepthSearch(position, depth), nil
}

func runFixedDepthSearch(position *board.Board, depth int) FixedDepthResult {
	clear()
	var local Local
	slInitEarly(&local, 0)
	slInitLate(&local)
	slSetRoot(&local, position)
	SetStop(false)
	ClearSearchCaches()
	initSg()
	var pv pvStruct
	score := search(&local, depth, minScore, maxScore, &pv)
	bestMove := move.None
	if pv.getSize() != 0 {
		bestMove = pv.getMove(0)
	}
	return FixedDepthResult{Score: score, BestMove: move.ToString(bestMove), Nodes: local.node}
}

// RunIterativeSiblingTrace runs the engine's normal root iterative deepening,
// including root move ordering and aspiration windows. TT/history are warmed
// by preceding iterations and only the final iteration is traced.
func RunIterativeSiblingTrace(position *board.Board, rootDepth, sourceIndex int, collector *SiblingTraceCollector) (FixedDepthResult, error) {
	if position == nil {
		return FixedDepthResult{}, errors.New("position is nil")
	}
	if collector == nil {
		return FixedDepthResult{}, errors.New("sibling trace collector is nil")
	}
	if rootDepth < 1 || rootDepth > maxDepth {
		return FixedDepthResult{}, fmt.Errorf("depth must be between 1 and %d", maxDepth)
	}
	if SearchStatus() == Running {
		return FixedDepthResult{}, errors.New("cannot run sibling diagnostic during an active search")
	}
	for depth := range collector.target {
		if depth > rootDepth {
			return FixedDepthResult{}, fmt.Errorf("target depth %d exceeds root depth %d", depth, rootDepth)
		}
	}
	previous := siblingTrace
	previousIterationHook := afterCompletedIteration
	defer func() {
		siblingTrace = previous
		afterCompletedIteration = previousIterationHook
		SetStop(false)
	}()
	collector.currentSource = sourceIndex
	siblingTrace = nil
	afterCompletedIteration = func(depth int) {
		if previousIterationHook != nil {
			previousIterationHook(depth)
		}
		if depth == rootDepth-1 {
			siblingTrace = collector
		}
	}
	if rootDepth == 1 {
		siblingTrace = collector
	}
	SetStop(false)
	ClearSearchCaches()
	NewSearch()
	SetMaxDepth(rootDepth)
	searchGo(position)
	result := FixedDepthResult{Score: Best.Score, BestMove: move.ToString(Best.move), Nodes: slEntries[0].node}
	collector.totalNodes += result.Nodes
	return result, nil
}

func recordSiblingTrace(bd *board.Board, depth, alpha, beta int, pvNode, inCheck, hardPruning bool, nodes int64) {
	c := siblingTrace
	if c == nil || !c.target[depth] {
		return
	}
	probe := SG.Trans.probeDiagnostic(hash.Key(bd.Key()), bd.Ply())
	bound := ""
	ttMove := ""
	if probe.Move != move.None {
		ttMove = move.ToString(probe.Move)
	}
	switch probe.Bound {
	case scoreTypeLower:
		bound = "lower"
	case scoreTypeUpper:
		bound = "upper"
	case scoreTypeBetween:
		bound = "between"
	}
	c.consider(SiblingTraceSnapshot{
		SchemaVersion: 2, SourceIndex: c.currentSource, FEN: bd.CreateFen(), PositionKey: uint64(bd.Key()),
		Depth: depth, Ply: bd.Ply(), Alpha: alpha, Beta: beta, PVNode: pvNode, InCheck: inCheck,
		HardPruning: hardPruning, RecaptureSquare: bd.Recap(), TTFound: probe.Found, TTDepthValid: probe.Found && probe.Depth >= depth,
		TTDepth: probe.Depth, TTMove: ttMove, TTScore: probe.Score, TTBound: bound, SearchNodes: nodes,
	})
}
