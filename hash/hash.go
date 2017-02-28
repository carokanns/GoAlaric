package hash

import (
	"GoAlaric/material"
	"GoAlaric/square"
	"fmt"
	"math/rand"
)

// Key is the Hash type
type Key uint64

//
const (
	WHITE int = iota
	BLACK
)

const stm int = material.SideSize * square.BoardSize
const castle int = stm + 1
const enPassant int = castle + 4
const size int = enPassant + 8

var randVal [size]Key

func rand64() Key {

	rand := Key(0)

	for i := 0; i < 4; i++ {
		rand = Key(int(rand<<16) | RandInt(1<<16))
	}

	return rand
}

func randKey(index int) Key {
	//assert(index >= 0 && index < SIZE);
	return randVal[index]
}

// StmKey computes the stm part of the hash key
func StmKey(turn int) Key {
	if turn == WHITE {
		return 0
	}
	return randKey(stm)
}

// FlagKey computes the castling part of the hash key
func FlagKey(flag uint) Key {
	//assert(flag < 4);
	return randKey(castle + int(flag))
}

// PieceKey computes the "piece on square" part of the hash key
func PieceKey(p12, sq int) Key {
	return randKey(p12*square.BoardSize + sq)
}

// StmFlip switch the key to the other stm
func StmFlip() Key {
	return randKey(stm)
}

// Init computes random values to the Hash random table
func init() {
	fmt.Println("info string Hash init startar")
	for i := 0; i < size; i++ {
		randVal[i] = rand64()
	}
}

// EnPassantKey computes the en passant part of the key
func EnPassantKey(sq int) Key {
	if sq == square.None {
		return 0
	}
	return randKey(enPassant + square.File(sq))

}

// Index computes the index to the table from the Key
func Index(key Key) int64 {
	return int64(key)
}

// Lock extracts the lock value from the hash key
func Lock(key Key) uint32 {
	return uint32(key >> 32)
}

// RandInt returns a random integer number
var r1 = (*rand.Rand)(rand.New(rand.NewSource(42)))

func RandInt(n int) int {
	//assert(n > 0);
	return r1.Intn(n)
}
