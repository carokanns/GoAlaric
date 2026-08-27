package board

import "testing"

func TestCreateFenPreservesCastlingRights(t *testing.T) {
	positions := []struct {
		name string
		fen  string
	}{
		{name: "white king side", fen: "4k3/8/8/8/8/8/8/R3K2R w K -"},
		{name: "white queen side", fen: "4k3/8/8/8/8/8/8/R3K2R w Q -"},
		{name: "black king side", fen: "r3k2r/8/8/8/8/8/8/4K3 b k -"},
		{name: "black queen side", fen: "r3k2r/8/8/8/8/8/8/4K3 b q -"},
		{name: "all rights", fen: "r3k2r/8/8/8/8/8/8/R3K2R w KQkq -"},
	}
	for _, test := range positions {
		t.Run(test.name, func(t *testing.T) {
			var original, decoded Board
			SetFen(test.fen, &original)
			encoded := original.CreateFen()
			SetFen(encoded, &decoded)
			if encoded != test.fen+" 0" {
				t.Fatalf("CreateFen() = %q, want %q", encoded, test.fen+" 0")
			}
			if decoded.Key() != original.Key() {
				t.Fatalf("round-trip key mismatch: encoded=%q", encoded)
			}
		})
	}
}
