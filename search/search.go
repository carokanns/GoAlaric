package search

import (
	"fmt"
	"math"
	"time"

	"goalaric/bit"
	"goalaric/board"
	"goalaric/eval"
	"goalaric/gen"
	"goalaric/hash"
	"goalaric/move"
)

var tellGUI = func(line string) { fmt.Println(line) }

// //// Engine paramters ///////
// status of the engine
const (
	idle    = 0
	Running = 1
)

const defaultHash = 128

type engineStruct struct {
	Hash    int
	Ponder  bool
	Threads int
	Log     bool
}

// Engine is the var holding engineStruct values
var Engine engineStruct

// Init the engine valuse
func init() {
	fmt.Printf("info string Engine init starts\n")
	Engine.Hash = defaultHash
	Engine.Ponder = false
	Engine.Threads = 1
	Engine.Log = false
}

////// Engine paramters END //////

// score types
const (
	noScore   = -10000
	minScore  = -9999
	maxScore  = +9999
	mateScore = +10000
)

const maxDepth = 100
const maxPly = 100
const nodeInterval = 1024
const maxThreads = 16
const maxQS = 2 // Max number of qs recursions

// Different types of handling for the go command from GUI
const (
	Simple int = iota
	Hard
	MateSearch
	Profiling
)

var genList [maxThreads][maxPly]gen.List
var genSearched [maxThreads][maxPly]gen.ScMvList
var genQS [maxThreads][maxQS]gen.SEE

type timerStr struct {
	timeT   time.Time
	elapsed int
	running bool
	startT  time.Time
}

func (t *timerStr) time() int {
	//ASSERT(t.p_running)
	now := time.Now()
	diff := now.Sub(t.startT)

	return int(diff.Nanoseconds() / 1000000) //gives milliseconds
}

func (t *timerStr) reset() {
	t.elapsed = 0
	t.running = false
}

func (t *timerStr) start() {
	t.startT = time.Now()
	t.running = true
}

func (t *timerStr) stop() {
	t.elapsed += t.time()
	t.running = false
}

func (t *timerStr) getElapsed() int {
	time := t.elapsed
	if t.running {
		time += t.time()
	}
	return time
}

// timeStr is the struct that holds search conditions
//
//	Exempel för att mäta tid om time Time
//	now := time.Now()
//	time.Sleep(100 * time.Millisecond)
//	now2 := time.Now()
//	diff := now2.Sub(now)
//	diff.Nanoseconds()
type limitStr struct {
	nodeIsLmited  bool // varför inte testa på om limit.nodes > 0
	timeIsLimited bool // varför inte testa på om limit.time > 0
	depth         int
	nodes         uint64
	time          uint64 // time limit for this move when we have X moves in T seconds
	hard          bool
	ponder        bool
	flag          bool
	step1         int64 // compute time limit in three steps
	stepB         int64 // compute time limit in three steps
	stepC         int64 // compute time limit in three steps
	lastScore     int   // last score we got
	drop          bool
	timer         timerStr // the timer
}

type currStruct struct {
	depth  int
	maxPly int
	node   int64
	time   int
	speed  int

	move     int
	pos      int
	size     int
	failHigh bool

	lastTime int
}

type bestStruct struct {
	depth     int
	move      int
	Score     int
	scoreType int
	pv        pvStruct
}

const pvSize int = maxPly

type pvStruct struct {
	size int
	move [pvSize]int
}

func (pv *pvStruct) mve(pos int) int {
	return pv.move[pos]
}
func (pv *pvStruct) clear() {
	pv.size = 0
}

func (pv *pvStruct) mvAdd(mv int) {
	if pv.size < pvSize {
		pv.move[pv.size] = mv
		pv.size++
	}
}

func (pv *pvStruct) add(npv *pvStruct) {
	for pos := 0; pos < npv.getSize(); pos++ {
		mv := npv.mve(pos)
		pv.mvAdd(mv)
	}
}
func (pv *pvStruct) getSize() int {
	return pv.size
}

func (pv *pvStruct) toString() string {
	s := ""
	for pos := 0; pos < pv.getSize(); pos++ {
		mv := pv.mve(pos)
		if pos != 0 {
			s += " "
		}
		s += move.ToString(mv)
	}

	return s
}
func (pv *pvStruct) catenate(mv int, npv *pvStruct) {
	pv.clear()
	pv.mvAdd(mv)
	pv.add(npv)
}
func (pv *pvStruct) getMove(pos int) int { // se även s_move(
	return pv.move[pos]
}

var limit limitStr
var current currStruct

// Best is the best move after each search
var Best bestStruct

var pv pvStruct

var bPonderHit bool
var bStop bool
var bQuit bool

// Status of the search middleGame/endGame
// Infinite is true when we search until stop from the GUI
var (
	Status   int
	Infinite bool
)

type searchGlobal struct { ////////: public util::Lockable {

	Trans   transTable
	History gen.HistoryTab
}

// SG includes the global data Hash table and Killer table that is not local (sl)
var SG searchGlobal

func sgAbort() {

	rootSP.updateRoot()

	for id := 0; id < Engine.Threads; id++ {
		// sl_signal(p_sl[id]); vänta in trådarna
	}
}

type splitPoint struct { ////////: public util::Lockable

	//private:

	master *Local
	parent *splitPoint

	board    board.Board
	depth    int
	oldAlpha int
	alpha    int // vill vara volatile - kan ge smp problem utan
	beta     int

	todo gen.ScMvList
	done gen.ScMvList

	workers  int // vill vara volatile - kan ge smp problem utan
	sent     int
	received int

	bestScore int // vill vara volatile - kan ge smp problem utan
	bestMove  int // vill vara volatile - kan ge smp problem utan
	pv        pvStruct
}

func (sp *splitPoint) initRoot(master *Local) {

	sp.master = master
	sp.parent = nil

	sp.bestScore = noScore
	sp.beta = maxScore
	sp.todo.Clear()

	sp.workers = 1
	sp.received = -1 // HACK
}

func (sp *splitPoint) updateRoot() {
	//lock()
	sp.received = 0
	sp.workers = 0
	//unlock()
}

var rootSP splitPoint

// Local is the data used for each thread
type Local struct {
	ID int
	//std::thread thread;

	todo   bool
	todoSP *splitPoint

	Board    board.Board
	killer   gen.Killer
	pawnHash eval.PawnHash
	evalHash eval.Hash
	node     int64 // vill vara volatile - kan ge smp problem utan
	maxPly   int   // vill vara volatile - kan ge smp problem utan

	mspStack     [16]splitPoint
	mspStackSize int

	sspStack     [64]*splitPoint // 64? verkligen? Kanske 16*4
	sspStackSize int
}

// ClearHash is used by tune.go
func (sl *Local) ClearHash() {
	sl.pawnHash.Clear()
	sl.evalHash.Clear()
}

var slEntries [maxThreads]Local

func (sl *Local) init() {
	for i := 0; i < 64; i++ {
		(sl.sspStack)[i] = new(splitPoint)
	}
}

// Init initialize and allocates hash tables once in the program startup
func init() {
	tellGUI("info string Search init startar")
	SG.Trans.InitTable()
	SG.Trans.SetSize(Engine.Hash)
	SG.Trans.Alloc()
	Status = idle
}

func clear() {
	limit.flag = false
	limit.timer.reset()
	limit.timer.start()

	current.depth = 0
	current.maxPly = 0
	current.node = 0
	current.time = 0
	current.speed = 0

	current.move = move.None
	current.pos = 0
	current.size = 0
	current.failHigh = false

	current.lastTime = 0

	Best.depth = 0
	Best.move = move.None
	Best.Score = noScore
	Best.scoreType = 0
	Best.pv.clear()
}

func updateBest(best *bestStruct, sc, scoreType int, pv *pvStruct) {

	//util.ASSERT(sc != None)
	//util.ASSERT(pv.size() != 0)

	limit.drop = scoreType == scoreTypeUpper || (sc <= limit.lastScore-30 && current.size > 1)

	if pv.getMove(0) != best.move || limit.drop {
		limit.flag = false
	}

	best.depth = current.depth
	best.move = pv.getMove(0)
	best.Score = sc
	best.scoreType = scoreType
	best.pv = *pv
}

func slSetRoot(sl *Local, bd *board.Board) {
	sl.Board = *bd
	sl.Board.SetRoot()
}
func undo(sl *Local) {
	bd := &sl.Board
	bd.Undo()
}

// NewSearch  initializes a new search
func NewSearch() {
	limit.nodeIsLmited = false
	limit.timeIsLimited = false
	limit.depth = maxDepth - 1

	limit.hard = false
	limit.ponder = false
}

// SetHard when the GUI sends go wtime/btime/winc/binc/movetogo  command
func SetHard(bd *board.Board, wtime, btime, winc, binc, mtg int64) {

	if mtg <= 0 {
		mtg = 50
	}
	mtg = int64(math.Min(float64(mtg), float64(eval.Interpolation(35, 15, bd))))
	time := wtime
	inc := winc
	if bd.Stm() == board.BLACK {
		time = btime
		inc = binc
	}
	total := time + inc*(mtg-1)
	factor := 120
	if Engine.Ponder {
		factor = 140
	}
	alloc := total / mtg * int64(factor) / 100
	reserve := total * (mtg - 1) / 50
	max := math.Min(float64(time), float64(total-reserve))
	max = math.Min(float64(max-60), float64(max*95/100)) // 60ms for lag

	alloc = int64(math.Max(float64(alloc), 0.0))
	max = math.Max(float64(max), 0.0)

	limit.hard = true
	limit.step1 = int64(math.Min(float64(alloc), float64(max)))
	limit.stepB = int64(math.Min(float64(alloc*4), float64(max)))
	limit.stepC = int64(max)
	limit.lastScore = noScore
}

// SetMaxDepth when the GUI sends the go depth command
func SetMaxDepth(d int) {
	if d <= 0 {
		limit.depth = maxDepth - 1
	} else {
		limit.depth = d
	}
}

// SetMaxNodes when the GUI sends the go nodes command
func SetMaxNodes(n uint64) {
	if n > 0 {
		limit.nodeIsLmited = true
		limit.nodes = n
	} else {
		limit.nodeIsLmited = false

	}
}

// SetMaxTime when the GUI sends the go movetime command
func SetMaxTime(t uint64) {
	if t > 0 {
		limit.timeIsLimited = true
		limit.time = t
	} else {
		limit.timeIsLimited = false
	}
}

// SetPonder (true) when the GUI send the go ponder command
func SetPonder(p bool) {
	limit.ponder = p
}

// StartSearch (called from GoAlaric.go) determines what kind of search that
// the GUI has demanded and sets values according that.
// It then starts up the search by calling search_go
//
//		Search kommer att polla på att bool bStop/bPonderHit är true
//		Ponderhit kan tas omhand genom att beräkna kvarvarande tid etc.
//		 - Om stop kommer skickar vi bestmove på den ponder som vi utför,
//	  - vilket ju kan vara fel drag
func StartSearch(searchType chan int, bestmove chan string, bd *board.Board) {
	//  Vi kommer att stå blockade här i väntan på return från RootSearch
	//  Uci tar hand om quit som bryter, stop och ponderhit skickas vidare
	//  till search.bStop resp bPonderHit
	if Status == Running {
		fmt.Println("StartSearch - already running.... error")
		return
	}
	for st := range searchType {

		switch st {
		case Simple: // infinite, depth, nodes, movetime
			Status = Running
			fmt.Println("info string simple search started!")
			searchGo(bd) // start search - polling for stop during search
		case Hard: // time is computed with increments. movestogo etc
			Status = Running
			fmt.Println("info string time constrained search started!")
			searchGo(bd) // start search - polling for stop during search
		case MateSearch:
			Status = Running
			//start search - polling for stop during search
			fmt.Println("info string mate search not yet ready! ")
		case Profiling:
			Status = idle
			return
		default:
			fmt.Printf("info string Invalid searchtype: %v", st)
			continue
		}

		writePV(&Best)
		bestmv := "bestmove " + move.ToString(Best.move)
		var ponder string
		if Best.pv.getSize() > 1 && Best.pv.getMove(1) != 0 {
			ponder = " ponder " + move.ToString(Best.pv.getMove(1))
		} else {
			ponder = ""
		}
		//	fmt.Println("bestscore ", best.score)
		SetStop(false)
		bestmove <- bestmv + ponder
		Status = idle
	}
}

// Search_go (called by StartSearch) initializes the search, the SMP and transposition table etc.
// Then it calls searchID. After the search is finished it up
func searchGo(bd *board.Board) {
	//	fmt.Println("Search_go")
	//	defer fmt.Println("exit Search_go")
	clear()
	initSg()

	for id := 0; id < Engine.Threads; id++ {
		slInitEarly(&slEntries[id], id)
	}

	rootSP.initRoot(&slEntries[0])

	for id := 1; id < Engine.Threads; id++ { // skip 0
		//p_sl[id].thread = std::thread(helper_program, &p_sl[id]);
	}

	slInitLate(&slEntries[0])

	searchID(bd)
	if bStop {
		sgAbort() // vänta in trådarna
	}

	for id := 1; id < Engine.Threads; id++ { // skip 0
		//          p_sl[id].thread.join();
	}

	searchEnd()
}

func initSg() {
	SG.History.Clear()
	SG.Trans.IncDate()
}

// searchID is the iterative deepening of the search.
// it adds one ply to the search for each loop unil time is up or some
// other search limit condition is met. It calls searchAsp
func searchID(bd *board.Board) {
	//	fmt.Println("search_id")
	//	defer fmt.Println("exit search_id")
	var sl = &slEntries[0]

	slSetRoot(sl, bd)
	slPush(sl, &rootSP)

	///// move generation /////

	var ml gen.ScMvList
	genAndSortLegals(sl, &ml) // generate legal and sort
	//util.ASSERT(ml.Size() != 0)
	Best.move = ml.Move(0)
	Best.Score = 0

	easy := (ml.Size() == 1 || (ml.Size() > 1 && ml.Score(0)-ml.Score(1) >= 50/4)) // HACK: uses gen_sort() internals
	easyMove := ml.Move(0)

	limit.lastScore = noScore

	///// iterative deepening /////
	for depth := 1; depth <= limit.depth; depth++ {
		depthStart(depth)
		searchAsp(&ml, depth)
		if bStop {
			return
		}
		//p_time.drop = (best.score <= p_time.last_score-50) // moved to update_best()
		limit.lastScore = Best.Score

		if Best.move != easyMove || limit.drop {
			easy = false
		}

		if limit.hard && !limit.drop {
			abort := false
			updateCurrent()

			if ml.Size() == 1 && int64(current.time) >= limit.step1/16 {
				abort = true
			}

			if easy && int64(current.time) >= limit.step1/4 {
				abort = true
			}

			if int64(current.time) >= limit.step1/2 {
				abort = true
			}

			if abort {
				if limit.ponder {
					limit.flag = true
				} else {
					bStop = true
					break
				}
			}
		}
		if bStop {
			break
		}
	}

	slPop(sl) //räkna in alla fåren
}

// search_asp makes the aspiration search. It calls searchRoot with
// a very narrow alpha-beta window and if the search returns a value outside the window
// it search again with a wider window. This goes on until the search
// returns a value that is inside the window
func searchAsp(ml *gen.ScMvList, depth int) {
	//	fmt.Println("search_asp depth", depth)
	//	defer fmt.Println("exit search_asp depth", depth)
	sl := &slEntries[0]

	//util.ASSERT(depth <= 1 || p_time.last_score == best.score)

	if depth >= 6 && !IsMateScore(limit.lastScore) {

		for margin := 10; margin < 500; margin *= 2 {

			a := limit.lastScore - margin
			b := limit.lastScore + margin
			//util.ASSERT(EVAL_MIN <= a && a < b && b <= EVAL_MAX)

			searchRoot(sl, ml, depth, a, b)
			if bStop {
				return
			}

			if Best.Score > a && Best.Score < b {
				return
			} else if IsMateScore(Best.Score) {
				break
			}
		}
	}

	searchRoot(sl, ml, depth, minScore, maxScore)
}

// search_root is the search from the current position.
// Here we can generate all the moves and sort them.
// Something that is not done deeper in the tree
func searchRoot(sl *Local, ml *gen.ScMvList, depth, alpha, beta int) {
	//	fmt.Println("search_root d=", depth)
	//	defer fmt.Println("exit search_root")
	//util.ASSERT(depth > 0 && depth < MAX_DEPTH)
	//util.ASSERT(alpha < beta)

	bd := &sl.Board

	pvNode := true

	bs := noScore
	bm := move.None
	oldAlpha := alpha

	// transposition table
	key := hash.Key(0)

	if depth >= 0 {
		key = bd.Key()
	}

	inCheck := eval.IsInCheck(bd)
	searchedSize := 0
	// move loop
	for pos := 0; pos < ml.Size(); pos++ {
		mv := ml.Move(pos)

		ext := extension(sl, mv, depth, pvNode)
		red := 0
		if ext == 0 {
			dangerous := inCheck || move.IsTactical(mv) || eval.IsCheck(mv, bd) || move.IsCastling(mv) || eval.IsPawnPush(mv, bd)
			red = reduction(sl, mv, depth /* pv_node,*/, inCheck, searchedSize, dangerous) // LMR
		}

		var sc int
		var npv pvStruct

		initMvSearch(mv, pos, ml.Size())

		slMove(sl, mv)
		//write_info()
		if !bStop {
			if (searchedSize == 0) && red != 0 {
				sc = -search(sl, depth-1+ext, -beta, -alpha, &npv)
			} else {
				sc = -search(sl, depth-1+ext-red, -alpha-1, -alpha, &npv)
				if sc > alpha { // PVS/LMR re-search
					failHighTrue()
					sc = -search(sl, depth-1+ext, -beta, -alpha, &npv)
				}
			}
		}
		undo(sl)
		failHighFalse()
		searchedSize++
		if bStop {
			return
		}

		if sc > bs {
			bs = sc
			var pv pvStruct

			pv.catenate(mv, &npv)
			updateBest(&Best, sc, scoreType(sc, alpha, beta), &pv)

			updateCurrent()
			writePV(&Best)

			if sc > alpha {

				bm = mv

				alpha = sc

				// ml.set_score(pos, sc); // not needed
				ml.MoveToFront(pos)

				///// Search_Global här
				if depth >= 0 {
					SG.Trans.Store(key, depth, bd.Ply(), mv, sc, scoreTypeLower)
				}

				if sc >= beta {
					return
				}
			}
		}
	}

	//util.ASSERT(bs != None)
	//util.ASSERT(bs < beta)

	if depth >= 0 {
		///// Search_Global här
		SG.Trans.Store(key, depth, bd.Ply(), bm, bs, scoreType(bs, oldAlpha, beta))
	}
}

// search is searching the nodes in all depths below the current position
// When it reaches its max search depth (set by search_go) it starts the
// qs (quiscense search) to make sure captures, checks and promotions are
// tried out before evaluation is made
func search(sl *Local, depth, alpha, beta int, pv *pvStruct) int {
	//	fmt.Println("search", depth, sl.board.Ply())
	//	defer fmt.Println("exit search", depth, sl.board.Ply())
	pv.clear()
	//util.ASSERT(depth < MAX_DEPTH, "depth=", depth)
	//util.ASSERT(alpha < beta, "alpha: ", alpha, "beta: ", beta)
	bd := &sl.Board
	//sc := alpha

	pvNode := depth > 0 && beta != alpha+1

	// mate-distance pruning

	if mateSc := AddMatePly(mateScore-1, bd.Ply()); mateSc < beta {

		beta = mateSc

		if mateSc <= alpha {
			return mateSc
		}
	}

	if bd.IsDraw() {
		return 0
	}

	stm := bd.Stm() // NOTE!! be aware of before and after move
	var attacks eval.Attacks
	eval.InitAttacks(&attacks, stm, bd)
	inCheck := attacks.Size != 0

	useTrans := depth >= 0
	transDepth := depth

	if depth < 0 && inCheck {
		useTrans = true
		transDepth = 0
	}

	key := hash.Key(0)
	transMove := move.None

	if useTrans {

		key = bd.Key() // It's here both castlingKey and epKey is included

		var transSc int
		var scoreType int

		if SG.Trans.Retrieve(key, transDepth, bd.Ply(), &transMove, &transSc, &scoreType) && !pvNode { // assigns trans_move #
			if scoreType == scoreTypeLower && transSc >= beta {
				return transSc
			}
			if scoreType == scoreTypeUpper && transSc <= alpha {
				return transSc
			}
			if scoreType == scoreTypeBetween {
				return transSc
			}
		}
	}

	ev := evalByColor(stm, sl)

	// ply limit
	if bd.Ply() >= maxPly || bStop {
		return ev
	}

	// beta pruning
	if !pvNode && depth > 0 && depth <= 3 && !IsMateScore(beta) && !inCheck {

		sc := ev - depth*50

		if sc >= beta {
			return sc
		}
	}

	// null-move pruning
	if !pvNode && depth > 0 && !IsMateScore(beta) && !inCheck && !board.LoneKing(stm, bd) && ev >= beta {

		bd.MoveNull() // TODO: use sl?

		sc := minScore

		if depth <= 3 { // static
			// if you don't beat me with 100 points,
			// then I think your position sucks
			sc = -Qs(sl, -beta+1, 100)
		} else { // dynamic
			var npv pvStruct
			sc = -search(sl, depth-3-1, -beta, -beta+1, &npv)
		}

		bd.UndoNull() // TODO: use sl?
		if sc >= beta {

			if useTrans {
				SG.Trans.Store(key, transDepth, bd.Ply(), move.None, sc, scoreTypeLower)
			}
			return sc
		}
		if bStop {
			return sc
		}
	}

	bs := noScore
	bm := move.None
	oldAlpha := alpha
	val := noScore // for delta pruning

	hardPruning := false // Futility and Delta and Material pruning

	if !inCheck { // rewritten apr 2018
		if depth <= 0 {
			bs = ev

			if ev > alpha {
				alpha = ev
				if alpha >= beta {
					return alpha
				}
			}
			hardPruning = true
			val = ev + 100 // QS-DP margin
		} else if depth <= 8 && !IsMateScore(alpha) {
			// futility-pruning condition
			sc := ev + depth*40

			if sc <= alpha {
				bs = sc
				hardPruning = true
			}

			val = sc + 50 // FP-DP margin, extra 50 for captures
		}
	}

	// IID
	if pvNode && depth >= 3 && transMove == move.None {

		var npv pvStruct
		sc := search(sl, depth-2, alpha, beta, &npv) // to keep PV-node property

		if sc > alpha && npv.getSize() != 0 {
			transMove = npv.getMove(0)
		}
	}

	//////// The move loop ///////////

	//var ml gen.List
	gl := &(genList[sl.ID][bd.Ply()])
	gl.Init(depth, bd, &attacks, transMove, &sl.killer, &SG.History, hardPruning)

	searched := &genSearched[sl.ID][bd.Ply()]
	searched.Clear()
	for mv := gl.Next(); mv != move.None; mv = gl.Next() {
		if hardPruning {
			if move.IsTactical(mv) && !eval.IsCheck(mv, bd) && val+move.CaptMax(mv) <= alpha { // delta pruning
				continue
			}
			if !gen.NoSacrifice(mv, bd) { // Material pruning
				continue
			}
		}

		dangerous := inCheck || move.IsTactical(mv) || eval.IsCheck(mv, bd) || move.IsCastling(mv) || eval.IsPawnPush(mv, bd) || gl.Candidate()

		if !pvNode && depth > 0 && depth <= 3 && !IsMateScore(bs) && searched.Size() >= depth*4 && !dangerous { // late-move pruning
			continue
		}

		ext := extension(sl, mv, depth, pvNode)

		red := 0
		if ext == 0 {
			red = reduction(sl, mv, depth /*pv_node,*/, inCheck, searched.Size(), dangerous) // LM Pruning
		}

		var sc int
		var npv pvStruct

		slMove(sl, mv) // do the move

		if (pvNode && searched.Size() != 0) || red != 0 {
			sc = -search(sl, depth+ext-red-1, -alpha-1, -alpha, &npv)
			if !bStop {
				if sc > alpha { // PVS/LMR re-search
					sc = -search(sl, depth+ext-1, -beta, -alpha, &npv)
				}
			}
		} else {
			sc = -search(sl, depth+ext-1, -beta, -alpha, &npv)
		}

		undo(sl) // undo the move

		//	fmt.Println("move loop end m", mv, bd.Ply(), i)
		//util.ASSERT(searched.Size() < sort.SIZE, "size är ", searched.Size())
		searched.Add(mv)
		if sc > bs {

			bs = sc
			pv.catenate(mv, &npv)

			if sc > alpha {

				bm = mv
				alpha = sc

				if useTrans {
					SG.Trans.Store(key, transDepth, bd.Ply(), mv, sc, scoreTypeLower)
				}

				if sc >= beta {
					if depth > 0 && !inCheck && !move.IsTactical(mv) {
						sl.killer.Add(mv, bd.Ply())
						SG.History.Add(mv, searched, bd)
					}
					return sc
				}
			}
		}
		if bStop {
			return alpha
		}

	} // end move loop

	if bs == noScore {
		if inCheck {
			return -mateScore + bd.Ply()
		}
		return 0
	}

	//util.ASSERT(bs < beta)

	if useTrans {
		SG.Trans.Store(key, transDepth, bd.Ply(), bm, bs, scoreType(bs, oldAlpha, beta))
	}

	return bs
}

func initMvSearch(mv, pos, size int) {

	// assert(size > 0);
	// assert(pos < size);
	current.move = mv
	current.pos = pos
	current.size = size

	current.failHigh = false
}
func failHighFalse() {
	current.failHigh = false
}
func failHighTrue() {
	current.failHigh = true
	limit.flag = false
}

// evalByColor evaluates a position and gives a value from side to move viewpoint.
// it doesn't check captures so that has to be done before eval starts.
func evalByColor(stm int, sl *Local) int {
	eval := sl.evalHash.Eval(&sl.Board, &sl.pawnHash)
	if stm == board.BLACK {
		return -eval
	}
	return eval
}

// Qs is the function called by the search when it is time to evaluate the position.
// This function makes sure that possible captures, promotions and checks are tried out first
// before the evaluation is done.
func Qs(sl *Local, beta, gain int) int { // for static NMP
	//fmt.Println("i Qs",parms.Parms[23],parms.Parms[24])
	//var se gen.SEE
	se := &(genQS[sl.ID][0]) // noll tills vi har en (=1) rekursion av qs
	//gl.Init(depth, bd, &attacks, trans_move, &sl.killer, &Sg.History, use_fp)
	bd := &sl.Board

	// assert(attack::is_legal(bd));
	// assert(!attack::is_in_check()); // triggers for root move ordering

	// stand pat
	bs := evalByColor(bd.Stm(), sl)
	val := bs + gain

	if bs >= beta {
		return bs
	}

	// move loop

	var attacks eval.Attacks
	eval.InitAttacks(&attacks, bd.Stm(), bd) // creates attacks - a bitboard with attacks to

	///// Search_Global här
	//var ml gen.List
	gl := &(genList[sl.ID][bd.Ply()])                                    // points to the current starting point in genlist
	gl.Init(-1, bd, &attacks, move.None, &sl.killer, &SG.History, false) // QS move generator

	done := bit.BB(0)

	for mv := gl.Next(); mv != move.None; mv = gl.Next() {

		if bit.IsOne(done, move.To(mv)) { // Don't do the same to-sq twice
			continue
		}

		bit.Set(&done, move.To(mv))

		see, cnt := se.See(mv, 0, EvalMAX, bd) // TODO: beta - val?
		if cnt > 0 {
			incNode(sl, cnt-1)
		}
		if see <= 0 {
			continue // don't consider equal captures as "threats"}
		}
		sc := val + see

		if sc > bs {

			bs = sc

			if sc >= beta {
				return sc
			}
		}
	}

	//util.ASSERT(bs < beta, "bs=", bs, "beta=", beta)
	return bs
}

func extension(sl *Local, mv int, depth int, pvNode bool) int {

	bd := &sl.Board

	if depth <= 4 && (eval.IsCheck(mv, bd) || gen.IsRecapture(mv, bd)) {
		return 1
	}

	if pvNode {
		if eval.IsCheck(mv, bd) || (move.IsTactical(mv) && gen.IsWin(mv, bd)) || eval.IsPawnPush(mv, bd) {
			return 1
		}
	}

	return 0

}

func reduction(sl *Local, mv int, depth int /* pvNode bool,*/, inCheck bool, searchedSize int, interesting bool) int {
	//int reduction(Search_Local & /* sl , int /* mv , int depth, bool /* pv_node , bool /* in_check , int searched_size, bool dangerous) {

	red := 0

	if depth >= 3 && searchedSize >= 3 && !interesting {
		red = 1
		if searchedSize >= 6 {
			red = depth / 3
		}
	}

	return red
}

func depthStart(depth int) {
	current.depth = depth
}

func updateCurrent() {

	node := int64(0)
	maxPly := 0

	for id := 0; id < Engine.Threads; id++ {

		sl := &slEntries[id]

		node += sl.node
		if sl.maxPly > maxPly {
			maxPly = sl.maxPly
		}
	}

	current.node = node
	current.maxPly = maxPly

	current.time = limit.timer.getElapsed()
	if current.time < 10 {
		current.speed = 0
	} else {

		current.speed = int(current.node * 1000 / int64(current.time))
	}

}

// SetStop (true) in order to stop search as quick as possible
func SetStop(s bool) {
	bStop = s
}
func searchEnd() {
	limit.timer.stop()
	updateCurrent()
	infoToGUI()
}

// incNode increments the node counter and checks if it's time to update
// current data for later info to GUI. It also checks if the time is up to stop.
// The cnt variable gives an interval 0-cnt within which the NODE_PERIOD test is true
func incNode(sl *Local, cnt int) {

	sl.node++

	if sl.node%nodeInterval <= int64(cnt) {

		abort := false

		updateCurrent()

		if poll() {
			abort = true
		}

		if limit.nodeIsLmited && uint64(current.node) >= limit.nodes {
			abort = true
		}

		if limit.timeIsLimited && uint64(current.time+3) >= limit.time {
			abort = true
		}

		if limit.hard && current.depth > 1 && int64(current.time) >= limit.step1 {
			if current.pos == 0 || int64(current.time) >= limit.stepB {
				if !(limit.drop || current.failHigh) || int64(current.time) >= limit.stepC {
					if limit.ponder {
						limit.flag = true
					} else {
						abort = true
					}
				}
			}
		}

		if limit.hard && current.depth > 1 && current.size == 1 && int64(current.time) >= limit.step1/8 {
			if limit.ponder {
				limit.flag = true
			} else {
				abort = true
			}
		}

		if abort {
			// Search_Global här
			sgAbort() // vänta in trådarna
			bStop = true
		}
	}

	//	if sl_stop(sl) {   // split logik
	//		Abort()
	//	}
}

func poll() bool {

	maybeToGUI() // if time exceeded, info to GUI

	//fmt.Println("\nsg.lock() ")
	//defer fmt.Println("sg.unlock() ")

	if bStop {
		//Infinite = false
		return true
	} else if bPonderHit {
		//Infinite = false
		limit.ponder = false
		return limit.flag
	}

	return false
}

// move(...) konfliktar så jag döper om till s_move
func slMove(sl *Local, mv int) {

	bd := &sl.Board

	incNode(sl, 0)
	bd.Move(mv)

	ply := bd.Ply()

	if ply > sl.maxPly {
		// assert(ply <= MAX_PLY);
		sl.maxPly = ply
	}
}

func genAndSortLegals(sl *Local, ml *gen.ScMvList) {

	var bd = &sl.Board

	gen.LegalMoves(ml, bd)

	v := evalByColor(bd.Stm(), sl)
	for pos := 0; pos < ml.Size(); pos++ {

		mv := ml.Move(pos)
		slMove(sl, mv)
		sc := -Qs(sl, maxScore, 0)

		undo(sl)

		sc = ((sc - v) / 4) + 1024 // HACK for unsigned 11-bit move-list scores
		//util.ASSERT(sc >= 0 && sc < move.SCORE_SIZE)

		ml.SetScore(pos, sc)
	}

	ml.Sort()
}

func maybeToGUI() {

	time := current.time

	if time >= current.lastTime+1000 {
		infoToGUI()
		current.lastTime = time - time%1000
	}
}

func infoToGUI() {

	//	fmt.Print("sg.lock()  ")
	//	defer fmt.Println("  sg.unlock()")
	line := fmt.Sprintf("info depth %v seldepth %v ", current.depth, current.maxPly)
	line += fmt.Sprintf("currmove %v ", move.ToString(current.move))
	line += fmt.Sprintf("currmovenumber %v ", current.pos+1)
	line += fmt.Sprintf("nodes %v ", current.node)
	line += fmt.Sprintf("time %v ", current.time)
	if current.speed != 0 {
		line += fmt.Sprintf("nps %v ", current.speed)
	}
	line += fmt.Sprintf("hashfull %v ", SG.Trans.Used())

	tellGUI(line)
}

// MateWithSign put +/- to a mate score
func mateWithSign(sc int) int {
	if sc < EvalMin { // -MATE
		return -(mateScore + sc) / 2
	} else if sc > EvalMAX { // +MATE
		return (mateScore - sc + 1) / 2
	}
	// assert(false);
	return 0
}

func writePV(best *bestStruct) {

	//	fmt.Println("sg.lock()")
	//	defer fmt.Println("sg.unlock()")

	line := fmt.Sprintf("info depth %v seldepth %v ", best.depth, current.maxPly)
	line += fmt.Sprintf("nodes %v time %v ", current.node, current.time)
	if IsMateScore(best.Score) {
		line += fmt.Sprintf(" score mate %v ", mateWithSign(best.Score))
	} else {
		line += fmt.Sprintf(" score cp %v ", best.Score)
	}

	if best.scoreType == scoreTypeLower {
		line += fmt.Sprintf("lowerbound ")
	}
	if best.scoreType == scoreTypeUpper {
		line += fmt.Sprintf("upperbound ")
	}

	line += fmt.Sprintf(" pv %v ", best.pv.toString())

	/*
	   std::cout << "info";
	   std::cout << " depth " << best.depth;
	   std::cout << " seldepth " << current.max_ply;
	   std::cout << " nodes " << current.node;
	   std::cout << " time " << current.time;

	   if (score::is_mate(best.score)) {
	      std::cout << " score mate " << score::signed_mate(best.score);
	   } else {
	      std::cout << " score cp " << best.score;
	   }
	   if (best.flags == score::flags_LOWER) std::cout << " lowerbound";
	   if (best.flags == score::flags_UPPER) std::cout << " upperbound";

	   std::cout << " pv " << best.pv.to_can();
	   std::cout << std::endl;
	*/

	tellGUI(line)
}

func slPush(sl *Local, sp *splitPoint) {
	//assert(sl.ssp_stack_size < 16);
	sl.sspStack[sl.sspStackSize] = sp
	sl.sspStackSize++
}
func slPop(sl *Local) {
	// assert(sl.ssp_stack_size > 0);
	sl.sspStackSize--
}

func slInitEarly(sl *Local, id int) {

	sl.ID = id

	sl.todo = true
	sl.todoSP = nil

	sl.node = 0
	sl.maxPly = 0

	sl.mspStackSize = 0
	sl.sspStackSize = 0
	sl.init()
}

func slInitLate(sl *Local) {
	sl.killer.Clear()
	sl.pawnHash.Clear() // pawn-eval cache
	sl.evalHash.Clear() // eval cache
}

// StartPerft starts the Perft command that generates all moves until the given depth.
// It counts the leafs only taht is printed out for each possible move from current pos
func StartPerft(depth int, bd *board.Board) uint64 {
	if depth <= 0 {
		fmt.Printf("Total:\t%v\n", 1)
		return 0
	}

	var ml gen.ScMvList

	gen.LegalMoves(&ml, bd)
	totCount := uint64(0)
	for pos := 0; pos < ml.Size(); pos++ {
		mv := ml.Move(pos)

		bd.Move(mv)
		count := perft(depth-1, bd)
		totCount += count
		fmt.Printf("%2d: %v \t%v\n", pos+1, move.ToString(mv), count)

		bd.Undo()
	}
	fmt.Println("------------------")
	fmt.Println()
	fmt.Printf("Total:\t%v\n", totCount)
	return totCount
}

func perft(depth int, bd *board.Board) uint64 {
	if depth == 0 {
		return 1
	}
	var ml gen.ScMvList
	gen.LegalMoves(&ml, bd)
	count := uint64(0)

	for pos := 0; pos < ml.Size(); pos++ {
		bd.Move(ml.Move(pos))
		count += perft(depth-1, bd)
		bd.Undo()
	}

	return count
}
