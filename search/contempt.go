package search

import (
	"fmt"

	"goalaric/board"
)

// Contempt values are expressed in centipawns. General Contempt applies to all
// draws and overrides SearchRepetitionContempt when non-zero.
const (
	DefaultContempt                 = 0
	DefaultSearchRepetitionContempt = 5
	MinContempt                     = -100
	MaxContempt                     = 100
)

// SetContempt validates and applies the user-controlled general draw contempt.
// Reconfiguration during a search is forbidden because workers read this value
// without synchronization.
func SetContempt(value int) error {
	if SearchStatus() == Running {
		return fmt.Errorf("Contempt cannot change during an active search")
	}
	if value < MinContempt || value > MaxContempt {
		return fmt.Errorf("Contempt must be between %d and %d", MinContempt, MaxContempt)
	}
	if Engine.Contempt == value {
		return nil
	}
	Engine.Contempt = value
	SG.Trans.Clear()
	return nil
}

// SetSearchRepetitionContempt changes the score used for early search
// repetitions while general Contempt is zero.
func SetSearchRepetitionContempt(value int) error {
	if SearchStatus() == Running {
		return fmt.Errorf("SearchRepetitionContempt cannot change during an active search")
	}
	if value < MinContempt || value > MaxContempt {
		return fmt.Errorf("SearchRepetitionContempt must be between %d and %d", MinContempt, MaxContempt)
	}
	if Engine.SearchRepetitionContempt == value {
		return nil
	}
	Engine.SearchRepetitionContempt = value
	SG.Trans.Clear()
	return nil
}

func repetitionContempt() int {
	if Engine.Contempt != DefaultContempt {
		return Engine.Contempt
	}
	return Engine.SearchRepetitionContempt
}

func drawScore(ply int) int {
	return rootRelativeContempt(Engine.Contempt, ply)
}

func repetitionScore(ply int) int {
	return rootRelativeContempt(repetitionContempt(), ply)
}

func drawStateScore(reason board.DrawReason, ply int) (int, bool) {
	switch reason {
	case board.DeadMaterialDraw, board.FiftyMoveDraw:
		return drawScore(ply), true
	case board.ThreefoldRepetition, board.SearchRepetition:
		return repetitionScore(ply), true
	default:
		return 0, false
	}
}

func rootRelativeContempt(contempt, ply int) int {
	if ply%2 == 0 {
		return -contempt
	}
	return contempt
}
