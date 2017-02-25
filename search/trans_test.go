// trans_test.go
package search

import (
	"GoAlaric/board"
	"GoAlaric/hash"
	"testing"
)

//var bd board.Board

//var sl Search_Local

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

func Test_Trans(t *testing.T) {
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
