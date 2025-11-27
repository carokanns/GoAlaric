// bit_test.go
package bit

import (
	"fmt"
	"testing"

	"goalaric/square"
)

func TestCount(t *testing.T) {
	type countStruct struct {
		bb    BB
		count int
	}

	var countTest = [...]countStruct{
		{BB(0), 0},
		{^BB(0), 64},
		{BB(1), 1},
		{BB(0x07), 3},
		{BB(0x7000000000000007), 6},
		{BB(0x7000000000000007), 6},
		{BB(0x7007000700070007), 15},
	}

	//initAll()
	for ix, tst := range countTest {
		if Count(tst.bb) != tst.count {
			t.Errorf("Testcase %v: Innehåller %v bits. Men blev %v", ix+1, tst.count, Count(tst.bb))
		}
	}
}

func ExampleFirst() {
	bb := BB(0x00001)      // note hex
	fmt.Println(First(bb)) // note binary
	bb = 0x10
	fmt.Println(First(bb))
	// Output:
	// 0
	// 4
}

func ExampleRest() {
	bb := BB(0x0011)             // note hex
	fmt.Printf("%b\n", Rest(bb)) // note binary
	bb = 0x0110
	fmt.Printf("%b", Rest(bb))
	// Output:
	// 10000
	// 100000000
}

// Example Rear shows how the bitboard is rotated 90 degrees to the right
func ExampleRear() {
	row := 1
	//InitBits()
	fmt.Printf("%064b\n", Rear(row))
	// Output:
	// 0000000100000001000000010000000100000001000000010000000100000001
}

func ExampleRearSd() {
	// NOTE: biboards are rotated left ( read BB from the right: sq==0 is A1, sq==1 is A2, sq==8 is B1, sq==63 is A8)
	sq := square.A2
	//InitBits()
	//	var bb BB
	//	Set(&bb, 2)
	//	Print_bb(bb)
	//	fmt.Printf("%064b\n", bb)
	fmt.Printf("%064b\n", RearSd(sq, WHITE))
	fmt.Printf("%b\n", RearSd(sq, BLACK))

	// Output:
	// 0000000100000001000000010000000100000001000000010000000100000001
	// 1111110011111100111111001111110011111100111111001111110011111100
}
func ExampleAdjFiles() {
	// NOTE: biboards are rotated left ( read BB from the right: sq==0 is A1, sq==1 is A2, sq==8 is B1, sq==63 is A8)
	file := 0
	//InitBits()
	fmt.Printf("%b\n", AdjFiles(file))

	// Output:
	// 1111111111111111
}

// copy of board function
/*func printBB(bb BB) {

	for rank := 7; rank >= 0; rank-- {
		fmt.Println(" ")
		for file := 0; file < square.FileSize; file++ {
			sq := square.Make(file, rank)
			if bb&Bit(sq) == 0 {
				fmt.Print("0 ")
			} else {
				fmt.Printf("1 ")
			}
		}
	}
	fmt.Printf("\n\n")
}*/
