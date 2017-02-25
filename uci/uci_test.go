// uci_test.go
package uci

import (
	"GoAlaric/search"
	"testing"
)

var chSearch = make(chan int)

func TestSetoption(t *testing.T) {
	HandleInput("setoption name Hash value 256", &chSearch)
	if search.Engine.Hash != 256 {
		t.Errorf("Hash borde vara %v men är %v", 256, search.Engine.Hash)
	}

}

func Test_GoCommand(t *testing.T) {

	HandleGo("go infinite test", &chSearch)
	if !search.Infinite {
		t.Errorf("Infinite borde vara satt till true men är false")
	}
}
