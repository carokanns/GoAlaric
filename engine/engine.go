package engine

import (
	"fmt"
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
