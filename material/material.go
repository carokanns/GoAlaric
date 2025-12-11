package material

import (
	"fmt"
	"math"
)

const pawnPhase = 0
const knightPhase = 1
const bishopPhase = 1
const rookPhase = 2
const queenPhase = 4

// TotalPhase is the phase weight for all pieces all together
const TotalPhase int = pawnPhase*16 + knightPhase*4 + bishopPhase*4 + rookPhase*4 + queenPhase*2

// PhaseWeight is the weight of a phase in evaluation
var PhaseWeight [TotalPhase + 1]int

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
