// uci_test.go
package uci

import (
	"testing"

	"goalaric/search"
)

var chSearch = make(chan int)

func TestSetoption(t *testing.T) {
	HandleInput("setoption name Hash value 256", &chSearch)
	if search.Engine.Hash != 256 {
		t.Errorf("Hash borde vara %v men är %v", 256, search.Engine.Hash)
	}
}

func TestSetoptionContempt(t *testing.T) {
	original := search.Engine.Contempt
	t.Cleanup(func() { search.Engine.Contempt = original })

	HandleInput("setoption name Contempt value 9", &chSearch)
	if search.Engine.Contempt != 9 {
		t.Errorf("Contempt borde vara %v men är %v", 9, search.Engine.Contempt)
	}

	HandleInput("setoption name Contempt value 101", &chSearch)
	if search.Engine.Contempt != 9 {
		t.Errorf("Ogiltigt Contempt-värde borde ignoreras, fick %v", search.Engine.Contempt)
	}
}

func Test_GoCommand(t *testing.T) {

	HandleGo("go infinite test", &chSearch)
	if !search.Infinite {
		t.Errorf("Infinite borde vara satt till true men är false")
	}
}
