package eval

import (
	"reflect"
	"strings"
	"testing"

	"goalaric/bit"
	"goalaric/board"
	"goalaric/material"
)

func TestPawnAttacksFrom(t *testing.T) {
	type args struct {
		bd *board.Board
	}
	tests := []struct {
		name string
		sd   int
		fen  string
		want bit.BB
	}{
		{"start", WHITE, board.StartFen, 0x0404040404040404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bd := &board.Board{}
			board.SetFen(strings.TrimSpace(tt.fen), bd)

			if got := PawnAttacksFrom(tt.sd, bd); !reflect.DeepEqual(got, tt.want) {
				board.PrintBB(got)
				fs := bd.PieceSd(material.Pawn, tt.sd)
				board.PrintBB(fs)
				t.Errorf("PawnAttacksFrom() = %x, want %x", got, tt.want)
			}
		})
	}
}
