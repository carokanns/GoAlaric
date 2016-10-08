// uci_test.go
package uci

import (
	"GoAlaric/engine"
	"GoAlaric/search"
	"testing"
)

var chSearch = make(chan int)

func TestSetoption(t *testing.T) {
	HandleInput("setoption name Hash value 234", &chSearch)
	if engine.Engine.Hash != 234 {
		t.Errorf("Hash borde vara %v men är %v", 234, engine.Engine.Hash)
	}
	HandleInput("setoption name Hash value 567", &chSearch)
	if engine.Engine.Hash != 567 {
		t.Errorf("Hash borde vara %v men är %v", 567, engine.Engine.Hash)
	}

}

func Test_GoCommand(t *testing.T) {

	HandleGo("go infinite test", &chSearch)
	if !search.Infinite {
		t.Errorf("Infinite borde vara satt till true men är false")
	}
}
