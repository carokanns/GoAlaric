// Package uci is the package for handling all uci requests from the GUI
// It communicates with the search; start, stop etc and receives bestmove from the search
// bestmove is sent to the GUI
package uci

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"goalaric/board"
	"goalaric/eval"
	"goalaric/gen"
	"goalaric/move"
	"goalaric/parms"
	"goalaric/search" //	"../sort"
	"goalaric/syzygy"
)

var tellGUI = func(line string) { fmt.Println(line) }
var trim = strings.TrimSpace
var tablebaseStatsGameStarted bool
var parameterFilePath string
var parameterFileSHA string

func init() {
	parameterFileSHA, _ = parms.DefaultParameterSHA256()
}

// Bd is the board on top of the whole session
var Bd board.Board

// HandleInput deals with all the different inputs from GUI
func HandleInput(line string, chSearch *chan int) string {
	line = strings.TrimSpace(line)
	words := strings.Split(line, " ")
	for len(words) > 0 {
		switch strings.ToLower(words[0]) {
		case "uci":
			tellGUI("id name GoAlaric")
			tellGUI("id author Peter Fendrich")

			tellGUI(fmt.Sprintf("option name Hash type spin default %v  min 16 max 16384", search.Engine.Hash))
			tellGUI(fmt.Sprintf("option name Ponder type check default %v", search.Engine.Ponder))
			tellGUI(fmt.Sprintf("option name Threads type spin default %v min 1 max 16", search.Engine.Threads))
			tellGUI(fmt.Sprintf("option name Contempt type spin default %d min %d max %d", search.DefaultContempt, search.MinContempt, search.MaxContempt))
			tellGUI(fmt.Sprintf("option name SearchRepetitionContempt type spin default %d min %d max %d", search.DefaultSearchRepetitionContempt, search.MinContempt, search.MaxContempt))
			tellGUI(fmt.Sprintf("option name SyzygyPath type string default %s", search.DefaultSyzygyPath))
			tellGUI(fmt.Sprintf("option name SyzygyProbeDepth type spin default %v min 1 max 100", search.Engine.SyzygyProbeDepth))
			tellGUI("option name TablebaseStatsFile type string default <empty>")
			tellGUI(fmt.Sprintf("option name LogFile type check default %v", search.Engine.Log))
			tellGUI("option name ParameterFile type string default <empty>")
			tellGUI(fmt.Sprintf("info string ParameterFile registry=%d sha256=%s source=builtin-defaults", parms.RegistryVersion, parameterFileSHA))

			tellGUI("uciok")
		case "isready":
			tellGUI("readyok")
		case "setoption":
			setOption(words[1:])
		case "position":
			SetPosition(line)
		case "go":
			HandleGo(line, chSearch)
		case "ponderhit":
			//tellGUI("ponderhit!")
		case "stop", "s":
			search.SetStop(true)
			return "stop"
		case "quit", "q":
			flushTablebaseGameStats()
			search.SetStop(true)
			return "quit"
		case "ucinewgame":
			initGame()
		case "debug":
			//tellGUI("debug!")
		case "register":

		///// Mina egna ///////////////////
		case "perft":
			if len(words) > 1 {
				depth, err := strconv.Atoi(words[1])
				if err != nil {
					tellGUI(err.Error())
				} else {
					search.StartPerft(depth, &Bd)
				}
			}
		case "pmirr": // make a mirror of the fen
			if len(words) > 1 {
				mfen := mirror(line[5:]) // line without 'pmirr'
				fmt.Println(mfen)
			}
		case "ptest":
			// check that the value after c0 in every epd-line is exactly eq to eval
			inputFile := "parmsGP.epd"
			_ = inputFile
			var epd []string
			_ = epd

			fmt.Println("scanEpd and runCompare are disabled as commented")
			//			scanEpd(inputFile,&epd) // läs in alla epd-positioner
			//			runGetParms(epd)
		case "pe":
			var pawnHash eval.PawnHash
			pawnHash.Clear()
			eval.Update()
			e := eval.CompEval(&Bd, &pawnHash) // NOTE: score for white
			tellGUI(fmt.Sprintf("eval(w): %v", e))
		case "pq": //qs value
			var sl search.Local
			search.SG.Trans.Clear()
			search.SG.History.Clear()
			sl.ClearHash()
			sl.ID = 0
			sl.Board = Bd
			eval.Update()
			val := search.Qs(&sl, search.EvalMAX, 0)
			if Bd.Stm() == eval.BLACK {
				val = -val
			}
			tellGUI(fmt.Sprintf("qs(w): %v", val))
		case "peng": // engine vslues
			txt := fmt.Sprintf("Hash: %v Threads: %v Ponder: %v Log: %v \n", search.Engine.Hash, search.Engine.Threads, search.Engine.Ponder, search.Engine.Log)
			tellGUI(txt)
		case "pb": // print  asci-board
			Bd.PrintBoard()
		case "pbb":
			Bd.PrintBBInfo() // all bitboards
		case "pm":
			PrintMoves() // all moves
		case "pf":
			PrintFens() // one fen per legal move in current position
		case "pn":
			endianCheck()
		case "help":
			tellGUI("case uci: Alltid vid start. Svarar med tellGUI(uciok)")
			tellGUI("case isready: Synkar med GUI:t som skall svarar med tellGUI(readyok)")

			tellGUI("case setoption: sätter options")
			tellGUI("case position:  sätter_position (fen)")
			tellGUI("case go: starta motorn")
			tellGUI("case ponderhit:")
			tellGUI("case stop, s: stoppa programmets sökning")
			tellGUI("case quit, q: stoppa he och lämna programmet")
			tellGUI("case ucinewgame: initGame()")
			tellGUI("case debug: //tellGUI(debug!)")
			tellGUI("case register:\n")

			tellGUI("///// Mina egna ///////////////////")
			tellGUI("case perft:  kör perft x (x ply)")
			tellGUI("case pmirr: // make a mirror of the fen")
			tellGUI("case ptest: // check that the value after c0 in every epd-line is exactly eq to eval")
			tellGUI("case pe:   pawn evaluation")
			tellGUI("case pq: //qs value")
			tellGUI("case peng: // engine values")
			tellGUI("case pb: // print  asci-board")
			tellGUI("case pbb: // printall bitboards")
			tellGUI("case pm: // all valid moves")
			tellGUI("case pf: // one fen per legal move in current position")
			tellGUI("case pn: endianCheck()")

		default:
			if len(words) > 1 {
				words = words[1:]
				continue
			}
		}
		return words[0]
	}
	return ""
}

/////////////////////////////////////////////////////////
/* func runGetParms(epd []string) {

	fmt.Println("starting test eval vs c0")
	fmt.Println()
	for pos := 0; pos < len(epd); pos++ {
		fields := strings.Fields(epd[pos])
		if fields[1] == "b" {
			epd[pos] = mirror(epd[pos])
		}

		SetPosition("position fen "+epd[pos])

		//get eval
		var pawnHash eval.PawnHash
		pawnHash.Clear()
		eval.Update()
		e := eval.CompEval(&Bd, &pawnHash) // NOTE: score for white

		// get c0 value
		c0,err := strconv.Atoi(fields[5])
		if err!=nil{log.Fatalf("illegal field %v line %v",fields[5],pos)}
		if c0 != e{
			fmt.Printf("ERROR: line %v epd-eval=%v eval=%v\n",pos,c0,e)
		}
		if pos%1000 == 0 {
			fmt.Println("\nPOS", pos, "====================== rootEv")
		}
	}
	fmt.Println("no of positions", len(epd))
}

func scanEpd(inputFile string, epd *[]string) {
	file, err := os.Open(inputFile)
	if err != nil {
		panic(err)
	}

	fscanner := bufio.NewScanner(file)
	ix := 0
	for fscanner.Scan() {
		epdLine := fscanner.Text()
		if len(epdLine) == 0 {
			continue
		}
		if ix = strings.Index(epdLine, "c0"); ix < 0 {
			log.Fatalln("c0 saknas i", epdLine)
		}

		*epd = append(*epd, epdLine)
	}
} */
////////////////////////////////////////////////////////////

func mirror(epd string) string {
	fields := strings.Fields(epd)
	if len(fields) < 4 {
		fmt.Println("invalid epd - missing data len=", len(fields), fields)
		return ""
	}

	rows := strings.Split(fields[0], "/")
	rows[0], rows[1], rows[2], rows[3], rows[4], rows[5], rows[6], rows[7] = rows[7], rows[6], rows[5], rows[4], rows[3], rows[2], rows[1], rows[0]
	temp := strings.Join(rows, "/")
	up := []string{"P", "N", "B", "R", "Q", "K"}
	lo := []string{"p", "n", "b", "r", "q", "k"}
	xd := []string{"@", "£", "$", "€", "<", ">"}
	for i := 0; i < len(up); i++ {
		temp = strings.Replace(temp, up[i], xd[i], -1)
	}
	for i := 0; i < len(lo); i++ {
		temp = strings.Replace(temp, lo[i], up[i], -1)
	}
	for i := 0; i < len(xd); i++ {
		temp = strings.Replace(temp, xd[i], lo[i], -1)
	}
	fields[0] = temp
	if fields[1] == "w" {
		fields[1] = "b"
	} else {
		fields[1] = "w"
	}
	if fields[2] != "-" {
		newField := ""
		if strings.Contains(fields[2][:], "k") {
			newField = newField + "K"
		}
		if strings.Contains(fields[2], "q") {
			newField = newField + "Q"
		}
		if strings.Contains(fields[2], "K") {
			newField = newField + "k"
		}
		if strings.Contains(fields[2], "Q") {
			newField = newField + "q"
		}
		//fmt.Printf("%v %#v\n", fields[2], newField)
		fields[2] = newField
	}

	if fields[3] != "-" {
		n, _ := strconv.Atoi(fields[3][1:])
		n = 9 - n
		fields[3] = fields[3][0:1] + strconv.Itoa(n)
	}

	newEpd := strings.Join(fields, " ")
	return newEpd
}

// HandleGo handles the go-command from GUI
func HandleGo(line string, chSearch *chan int) {
	if search.SearchStatus() == search.Running {
		tellGUI("info string Inte en go till innan search klar")
		return
	}
	search.NewSearch()
	tablebaseStatsGameStarted = true

	line = strings.ToLower(strings.TrimSpace(line))

	if strings.Contains(line, "infinite") {
		search.SetInfinite(true)
	}

	if strings.Contains(line, "ponder") {
		search.SetPonder(true)
	}

	//	if strings.Index(line, "searchmoves ") >= 0 {
	//		pickup the moves
	//		search.SetSearchMoves(.....)
	//	}

	/// hard part starts ///
	bHard, found := false, false
	var wtime, btime, winc, binc, mtg int64 = 0, 0, 0, 0, 0
	if wtime, found = pickNumber(line, "wtime "); found {
		bHard = true
	}
	if btime, found = pickNumber(line, "btime "); found {
		bHard = true
	}
	if winc, found = pickNumber(line, "winc "); found {
		bHard = true
	}
	if binc, found = pickNumber(line, "binc "); found {
		bHard = true
	}
	if mtg, found = pickNumber(line, "movestogo "); found {
		bHard = true
	}
	if bHard {
		search.SetHard(&Bd, wtime, btime, winc, binc, mtg)
		/// hard part end ///
	} else {
		// strange GUI command if bHard and it continues here. So we just ignores it
		if movetime, found := pickNumber(line, "movetime "); found {
			search.SetMaxTime(uint64(movetime))
		} else {
			search.SetMaxTime(uint64(0)) // default if error in GUI line
		}

		if depth, found := pickNumber(line, "depth "); found {
			search.SetMaxDepth(int(depth))
		} else {
			search.SetMaxDepth(0) // default if error in GUI depth
		}

		if nodes, found := pickNumber(line, "nodes "); found {
			search.SetMaxNodes(uint64(nodes))
		} else {
			search.SetMaxNodes(uint64(0)) // default if error in GUI nodes
		}

		//	if mate, found := pickNumber(line, "mate "); found {
		//		search.SetMate(mate)
		//	}
	}
	if !strings.HasSuffix(line, "test") { // if test skip the search!
		if bHard {
			*chSearch <- search.Hard
		} else {
			*chSearch <- search.Simple
		}
	}
}

func pickNumber(line string, cmd string) (int64, bool) {
	ix := strings.Index(line, cmd)
	if ix > 0 {
		rest := strings.Split(line[ix+len(cmd):], " ")
		number, err := strconv.Atoi(trim(rest[0]))
		if err == nil {
			return int64(number), true
		}

	}
	return 0, false

}

func setOption(words []string) { // NOTE: "setoption" is already removed from the parameter words
	if len(words) < 2 || !strings.EqualFold(strings.TrimSpace(words[0]), "name") {
		return
	}
	valueAt := -1
	for i := 1; i < len(words); i++ {
		if strings.EqualFold(strings.TrimSpace(words[i]), "value") {
			valueAt = i
			break
		}
	}
	if valueAt < 2 {
		return
	}
	word := strings.ToLower(strings.Join(words[1:valueAt], " "))
	value := strings.TrimSpace(strings.Join(words[valueAt+1:], " "))
	switch word {
	case "hash":
		fmt.Println("info string Hash before:", search.Engine.Hash)
		hashVal, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return
		}
		newHash := int(hashVal)
		if newHash == search.Engine.Hash {
			// Same size requested: clear entries because the hash setting was reset.
			search.SG.Trans.Clear()
		} else {
			search.Engine.Hash = newHash
			search.SG.Trans.Resize(search.Engine.Hash)
		}

		fmt.Println("info string Hash after:", search.Engine.Hash)

	case "threads":
		threads, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return
		}
		search.Engine.Threads = int(threads)
	case "contempt":
		contempt, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			tellGUI(fmt.Sprintf("info string Contempt rejected: %v", err))
			return
		}
		if err := search.SetContempt(int(contempt)); err != nil {
			tellGUI(fmt.Sprintf("info string Contempt rejected: %v", err))
		}
	case "searchrepetitioncontempt":
		contempt, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			tellGUI(fmt.Sprintf("info string SearchRepetitionContempt rejected: %v", err))
			return
		}
		if err := search.SetSearchRepetitionContempt(int(contempt)); err != nil {
			tellGUI(fmt.Sprintf("info string SearchRepetitionContempt rejected: %v", err))
		}
	case "syzygypath":
		path := syzygy.NormalizePath(value)
		if strings.EqualFold(path, "off") {
			path = ""
		}
		largest, err := search.SetSyzygyPath(path)
		if err != nil {
			tellGUI("info string Syzygy disabled: " + err.Error())
			return
		}
		if path == "" {
			tellGUI("info string Syzygy disabled")
		} else {
			tellGUI(fmt.Sprintf("info string Syzygy loaded: path=%s pieces=%d", path, largest))
		}
	case "syzygyprobedepth":
		depth, err := strconv.ParseInt(value, 10, 32)
		if err != nil || depth < 1 || depth > 100 {
			return
		}
		search.Engine.SyzygyProbeDepth = int(depth)
		search.SG.Trans.Clear()
	case "tablebasestatsfile":
		search.Engine.TablebaseStatsFile = value
	case "log", "logfile":
		log, err := strconv.ParseBool(value)
		if err != nil {
			return
		}
		search.Engine.Log = log
	case "parameterfile":
		setParameterFile(value)
	case "ponder":
		ponder, err := strconv.ParseBool(value)
		if err != nil {
			return
		}
		search.Engine.Ponder = ponder
	}
}

func setParameterFile(path string) {
	if search.SearchStatus() == search.Running {
		tellGUI("info string ParameterFile rejected: search active")
		return
	}

	path = strings.TrimSpace(path)
	file := parms.DefaultParameterFile()
	digest := parameterFileSHA
	source := "builtin-defaults"
	if path != "" {
		loaded, loadedDigest, err := parms.LoadParameterFile(path)
		if err != nil {
			tellGUI("info string ParameterFile rejected: " + err.Error())
			return
		}
		file = loaded
		digest = loadedDigest
		source = path
	}

	if err := eval.ApplyParameterFile(file); err != nil {
		tellGUI("info string ParameterFile rejected: " + err.Error())
		return
	}
	search.RefreshRuntimeParameters()
	search.ClearSearchCaches()
	parameterFilePath = path
	parameterFileSHA = digest
	tellGUI(fmt.Sprintf("info string ParameterFile loaded: registry=%d registry_name=%s sha256=%s source=%s", file.SchemaVersion, file.Registry, digest, source))
}

// SetPosition sets up a position from the GUI position command (see UCI spec)
func SetPosition(str string) {
	parts := strings.Split(str, " ")
	if len(parts) < 2 {
		return // nothing to do on bare "position"
	}
	words := parts[1:]
	word1 := strings.ToLower(strings.TrimSpace(words[0]))
	mpos := strings.Index(strings.TrimSpace(str), "moves")

	switch word1 {
	case "startpos":
		board.SetFen(board.StartFen, &Bd)
	case "fen":
		if mpos == -1 {
			if len(words) < 2 { // missing fen payload
				return
			}
			board.SetFen(strings.Join(words[1:], " "), &Bd)
		} else {
			fpos := strings.Index(str, "fen") + 4
			board.SetFen(strings.TrimSpace(str[fpos:mpos]), &Bd)
		}
	default:
		return
	}

	if mpos >= 0 {

		moves := strings.Split(str[mpos+5:], " ")

		var ml gen.ScMvList
		var movesUse []string

		// check if the moves are legal. Need to actually update the board
		for _, strMve := range moves {
			if strings.TrimSpace(strMve) == "" {
				continue
			}

			ml.Clear()
			gen.LegalMoves(&ml, &Bd)
			if isLegal(strMve, &ml) {
				movesUse = append(movesUse, strings.TrimSpace(strMve))
				mve := board.FromString(strMve, &Bd)
				Bd.MakeFenMve(mve) // gör draget för att testa nästa
			} else {
				break
			}
		}
		SetPosition(str[:mpos]) // Återställ brädet utan 'moves'

		board.FenMoves(movesUse, &Bd)
	}
}

func isLegal(strMve string, ml *gen.ScMvList) bool {
	strMve = strings.ToLower(strings.TrimSpace(strMve))

	for ix := 0; ix < ml.Size(); ix++ {
		strLegal := move.ToString(ml.Move(ix))
		if strMve == strings.ToLower(strings.TrimSpace(strLegal)) {
			return true
		}
	}
	return false
}

// initGame görs efter varje ucinewgame
func initGame() {
	flushTablebaseGameStats()
	SetPosition("position startpos")
	search.SG.Trans.Clear()
	search.SG.History.Clear()
}

func flushTablebaseGameStats() {
	if !tablebaseStatsGameStarted {
		return
	}
	tablebaseStatsGameStarted = false
	hits, rootWins := search.ConsumeTablebaseGameStats()
	path := search.Engine.TablebaseStatsFile
	if path == "" {
		return
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(file, "hits=%d root_wins=%d\n", hits, rootWins)
	_ = file.Close()
}

//////////////// för testande /////////////////////

// PrintMoves is for testing. It prints all legal moves in the current position
func PrintMoves() {
	var ml gen.ScMvList
	gen.LegalMoves(&ml, &Bd)
	gen.PrintAllMoves(&ml)
}

// PrintFens is for GPTune test. It prints fen from all legal moves in the current position
func PrintFens() {
	var ml gen.ScMvList
	gen.LegalMoves(&ml, &Bd)
	for pos := 0; pos < ml.Size(); pos++ {
		mv := ml.Move(pos)
		strMove := move.ToString(mv)
		Bd.Move(mv)
		epd := Bd.CreateFen()

		fmt.Println(epd, "c0 0 c1 1 c2", strMove)
		Bd.Undo()
	}
}

// endianCheck is a test function to determine if the processor is using Big or Low Endian
func endianCheck() {
	var TellGUI = func(line string) {
		fmt.Println(line)
	}
	var ourOrderIsBE bool

	// case will need maintenace with more BE archs coming

	switch runtime.GOARCH {
	case "mips64", "ppc64":
		ourOrderIsBE = true
	}

	if ourOrderIsBE {
		TellGUI("info string BigEndian")
	} else {
		TellGUI("info string LowEndian")
	}
}
