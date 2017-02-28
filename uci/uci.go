// Package uci is the package for handling all uci requests from the GUI
// It communicates with the search; start, stop etc and receives bestmove from the search
// bestmove is sent to the GUI
package uci

import (
	"GoAlaric/board"
	"GoAlaric/eval"
	"GoAlaric/gen"
	"GoAlaric/move"
	"GoAlaric/search"
	//	"GoAlaric/sort"
	"fmt"
	"runtime"
	"strconv"
	"strings"
)

var tellGUI = func(line string) { fmt.Println(line) }
var trim = strings.TrimSpace

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

			tellGUI(fmt.Sprintf("option name Hash type spin default %v  min 16 max 16384\n", search.Engine.Hash))
			tellGUI(fmt.Sprintf("option name Ponder type check default %v\n", search.Engine.Ponder))
			tellGUI(fmt.Sprintf("option name Threads type spin default %v min 1 max 16\n", search.Engine.Threads))
			tellGUI(fmt.Sprintf("option name LogFile type check default %v\n", search.Engine.Log))

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
		case "pe":
			var pawnHash eval.PawnHash
			pawnHash.Clear()

			e := eval.CompEval(&Bd, &pawnHash) // NOTE: score for white
			tellGUI(fmt.Sprintf("eval(w): %v", e))
		case "peng":
			txt := fmt.Sprintf("Hash: %v Threads: %v Ponder: %v Log: %v \n", search.Engine.Hash, search.Engine.Threads, search.Engine.Ponder, search.Engine.Log)
			tellGUI(txt)
		case "pb":
			Bd.PrintBoard()
		case "pbb":
			Bd.PrintBBInfo()
		case "pm":
			PrintMoves()
		case "pn":
			endianCheck()
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

/*
 go
	start calculating on the current position set up with the "position" command.
	There are a number of commands that can follow this command, all will be sent in the same string.
	If one command is not sent its value should be interpreted as it would not influence the search.
	* searchmoves <move1> .... <movei>
		restrict search to this moves only
		Example: After "position startpos" and "go infinite searchmoves e2e4 d2d4"
		the engine should only search the two moves e2e4 and d2d4 in the initial position.
	* ponder
		start searching in pondering mode.
		Do not exit the search in ponder mode, even if it's mate!
		This means that the last move sent in in the position string is the ponder move.
		The engine can do what it wants to do, but after a "ponderhit" command
		it should execute the suggested move to ponder on. This means that the ponder move sent by
		the GUI can be interpreted as a recommendation about which move to ponder. However, if the
		engine decides to ponder on a different move, it should not display any mainlines as they are
		likely to be misinterpreted by the GUI because the GUI expects the engine to ponder
	   on the suggested move.
	* wtime <x>
		white has x msec left on the clock
	* btime <x>
		black has x msec left on the clock
	* winc <x>
		white increment per move in mseconds if x > 0
	* binc <x>
		black increment per move in mseconds if x > 0
	* movestogo <x>
      there are x moves to the next time control,
		this will only be sent if x > 0,
		if you don't get this and get the wtime and btime it's sudden death
	* depth <x>
		search x plies only.
	* nodes <x>
	   search x nodes only,
	* mate <x>
		search for a mate in x moves
	* movetime <x>
		search exactly x mseconds
	* infinite
		search until the "stop" command. Do not exit the search without being told so in this mode!
*/

// HandleGo handles the go-command from GUI
func HandleGo(line string, chSearch *chan int) {
	if search.Status == search.Running {
		tellGUI("info string Inte en go till innan search klar")
		return
	}
	search.NewSearch()

	line = strings.ToLower(strings.TrimSpace(line))

	if strings.Index(line, "infinite") >= 0 {
		search.Infinite = true
	}

	if strings.Index(line, "ponder") >= 0 {
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
	word := strings.ToLower(strings.TrimSpace(words[0]))
	if word != "name" {
		return
	}
	if len(words) < 4 {
		return
	}
	word = strings.ToLower(strings.TrimSpace(words[2]))
	if word != "value" {
		return
	}

	word = strings.ToLower(strings.TrimSpace(words[1]))
	value := strings.ToLower(strings.TrimSpace(words[3]))
	switch word {
	case "hash":
		fmt.Println("info string Hash before:", search.Engine.Hash)
		hashVal, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return
		}
		search.Engine.Hash = int(hashVal)
		search.SG.Trans.InitTable()
		search.SG.Trans.SetSize(search.Engine.Hash)
		search.SG.Trans.Alloc()

		fmt.Println("info string Hash after:", search.Engine.Hash)

	case "threads":
		threads, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return
		}
		search.Engine.Threads = int(threads)
	case "log":
		log, err := strconv.ParseBool(value)
		if err != nil {
			return
		}
		search.Engine.Log = log
	case "ponder":
		ponder, err := strconv.ParseBool(value)
		if err != nil {
			return
		}
		search.Engine.Ponder = ponder
	}
}

// SetPosition sets up a position from the GUI position command (see UCI spec)
func SetPosition(str string) {
	words := strings.Split(str, " ")[1:]
	word1 := strings.ToLower(strings.TrimSpace(words[0]))
	mpos := strings.Index(strings.TrimSpace(str), "moves")
	if word1 == "startpos" {
		board.SetFen(board.StartFen, &Bd)
	} else if word1 == "fen" {
		if mpos == -1 {
			board.SetFen(strings.Join(words[1:], " "), &Bd)
		} else {
			fpos := strings.Index(str, "fen") + 4
			board.SetFen(strings.TrimSpace(str[fpos:mpos]), &Bd)
		}
	} else {
		return
	}
	if mpos >= 0 {

		moves := strings.Split(str[mpos+5:], " ")

		var ml gen.ScMvList
		strUse := ""

		// check if the moves are legal. Need to actually update the board
		for _, strMve := range moves {
			if strings.TrimSpace(strMve) == "" {
				continue
			}

			ml.Clear()
			gen.LegalMoves(&ml, &Bd)
			if isLegal(strMve, &ml) {
				strUse += strMve + " "
				mve := board.FromString(strMve, &Bd)
				Bd.MakeFenMve(mve) // gör draget för att testa nästa
			} else {
				break
			}
		}
		SetPosition(str[:mpos]) // Återställ brädet utan 'moves'

		movesUse := strings.Split(strUse, " ")
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
	SetPosition("position startpos")
	search.SG.Trans.Clear()
	search.SG.History.Clear()
}

//////////////// för testande /////////////////////

// PrintMoves is for testing. It prints all the move in the current position
func PrintMoves() {
	var ml gen.ScMvList
	gen.LegalMoves(&ml, &Bd)
	gen.PrintAllMoves(&ml)
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
