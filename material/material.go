package material

import (
	"fmt"
	//"GoAlaric/eval"
	//	"GoAlaric/board" //går ej!
	"GoAlaric/piece"
	"math"
)

//

const stageSize int = 2 // NOTE: declared elsewhere to
const pawnPhase int = 0
const knightPhase int = 1
const bishopPhase int = 1
const rookPhase int = 2
const queenPhase int = 4

// TotalPhase is the phase weight for all pieces all together
const TotalPhase int = pawnPhase*16 + knightPhase*4 + bishopPhase*4 + rookPhase*4 + queenPhase*2

// PhaseWeight ir the weight of a phase in evaluation
var PhaseWeight [TotalPhase + 1]int

var score = [piece.Size][stageSize]int{{85, 95}, {325, 325}, {325, 325}, {460, 540}, {975, 975}, {0, 0}, {0, 0}}
var powerVal = [piece.Size]int{0, 1, 1, 2, 4, 0, 0}

var phaseVal = [...]int{pawnPhase, knightPhase, bishopPhase, rookPhase, queenPhase, 0, 0}

// Phase returns the phase value to use for the piece
func Phase(pc int) int {
	//assert(pc < piece::SIZE);
	return phaseVal[pc]
}

// Init material stuff (weight of the different phases
func init() {
	fmt.Println("info string material init starts")
	for i := 0; i <= TotalPhase; i++ {
		x := float64(i)/float64(TotalPhase/2) - 1.0
		y := 1.0 / (1.0 + math.Exp(float64(-x*5.0)))
		PhaseWeight[i] = int(y * 256.0)
	}
}

// Power returns the power of a piece in capture situations
func Power(pc int) int {
	//util.ASSERT(pc < piece.SIZE)
	return powerVal[pc]
}

// Score returns piece score for stage
func Score(pc, stage int) int {
	//util.ASSERT(pc < piece.SIZE)
	//util.ASSERT(stage < Stage_SIZE)
	return score[pc][stage]
}
