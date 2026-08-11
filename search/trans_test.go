// trans_test.go testar transpositionstabellen.
package search

import (
	"reflect"
	"testing"

	"goalaric/board"
	"goalaric/hash"
)

func TestTransEntrySizeMatchesAllocationConstant(t *testing.T) {
	got := int(reflect.TypeOf(entry{}).Size())
	if got != sizeEntry {
		t.Fatalf("entry size = %d bytes, sizeEntry = %d", got, sizeEntry)
	}
}

// initAll är en stub för att spegla initSession i main när det behövs i tester.
func initAll() { // copy of main initSession()
	//	input.Init()
	//engine.Init()
	//material.Init()
	//eval.PstInit()
	//eval.PawnInit()
	//eval.Init()
	//search.Init()
	//bit.InitBits()
	//hash.Init()
	//castling.Init()
	//eval.AtkInit()
}

func TestTrans(t *testing.T) {
	// Verifierar lagring, ersättning och återläsning av TT-poster.
	initAll()
	board.SetFen("8/6kp/5p2/3n2pq/3N1n1R/1P3P2/P6P/4QK2 w - - 2 2", &bd)
	var hashTab transTable
	hashTab.InitTable()
	hashTab.SetSize(64)
	hashTab.Alloc()
	type transStruct struct {
		key     hash.Key
		mv      int
		depth   int
		ply     int
		sc      int
		flags   int
		comment string
	}

	var transTest = [...]transStruct{
		// key               mv  d   p  sc  fl
		{0x0fffffffffffffff, 10, 10, 5, 1, 0xf, "first entry"},
		{0x1fffffffffffffff, 20, 9, 5, 2, 0xf, "second entry"},
		{0x2fffffffffffffff, 30, 6, 5, 3, 0xf, "third entry should be replaced by fifth"},
		{0x3fffffffffffffff, 40, 7, 5, 4, 0xf, "fourth entry"},
		{0x4fffffffffffffff, 50, 11, 5, 5, 0xf, "fifth entry replaces the third"},
	}
	for _, e := range transTest {
		mv := e.mv
		key := e.key
		depth := e.depth
		ply := e.ply
		sc := e.sc
		flags := e.flags
		hashTab.Store(key, depth, ply, mv, sc, flags)
	}

	for ix, e := range transTest {
		mv := e.mv
		key := e.key
		depth := e.depth
		ply := e.ply
		sc := e.sc
		flags := e.flags
		rmv := 9999
		rsc := 9999
		rflags := 9999

		if hashTab.Retrieve(key, depth, ply, &rmv, &rsc, &rflags) {
			if rmv == mv && rsc == sc && rflags == flags {
				if ix == 2 {
					t.Errorf("case %v: %v", ix+1, e.comment)
				} else {
					//ok
				}
			} else {
				t.Errorf("case %v: values not ok. (mv %v,rmv %v), (sc %v,rsc %v), (flags %v, rflags %v\n", ix+1, mv, rmv, sc, rsc, flags, rflags)
			}
		} else {

			if ix == 2 {
				//ok
			} else {
				t.Errorf("case %v: couldn't find the entry", ix+1)
			}
		}
	}
}

func TestTransDoesNotReviveDeeperEntryFromOldGeneration(t *testing.T) {
	var tt transTable
	tt.InitTable()
	tt.SetSize(1)
	tt.Alloc()

	key := hash.Key(0x123456789abcdef)
	tt.Store(key, 10, 0, 10, 123, scoreTypeBetween)

	tt.IncDate()
	tt.Store(key, 1, 0, 20, 45, scoreTypeBetween)

	var mv, sc, flags int
	if tt.Retrieve(key, 10, 0, &mv, &sc, &flags) {
		t.Fatalf("old depth-10 entry was revived: move=%d score=%d flags=%d", mv, sc, flags)
	}

	if !tt.Retrieve(key, 1, 0, &mv, &sc, &flags) {
		t.Fatal("new depth-1 entry was not stored")
	}
	if mv != 20 || sc != 45 || flags != scoreTypeBetween {
		t.Fatalf("new entry = move %d score %d flags %d", mv, sc, flags)
	}
}

func TestTransGenerationWrapDoesNotReviveOldEntry(t *testing.T) {
	var tt transTable
	tt.InitTable()
	tt.SetSize(1)
	tt.Alloc()

	key := hash.Key(0x23456789abcdef1)
	tt.Store(key, 10, 0, 10, 123, scoreTypeBetween)

	for i := 0; i < 256; i++ {
		tt.IncDate()
	}

	var mv, sc, flags int
	if tt.Retrieve(key, 10, 0, &mv, &sc, &flags) {
		t.Fatalf(
			"entry revived after generation wrap: move=%d score=%d flags=%d",
			mv, sc, flags,
		)
	}
}

func TestTransClearsTableAtUint16GenerationWrap(t *testing.T) {
	var tt transTable
	tt.InitTable()
	tt.SetSize(1)
	tt.Alloc()

	tt.generation = 65535

	key := hash.Key(0x3456789abcdef12)
	tt.Store(key, 10, 0, 10, 123, scoreTypeBetween)
	tt.IncDate()

	if tt.generation != 1 {
		t.Fatalf("generation after wrap = %d, want 1", tt.generation)
	}

	var mv, sc, flags int
	if tt.Retrieve(key, 10, 0, &mv, &sc, &flags) {
		t.Fatalf(
			"entry survived physical generation clear: move=%d score=%d flags=%d",
			mv, sc, flags,
		)
	}
}
