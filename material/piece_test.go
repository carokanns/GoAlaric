package material

import (
	"testing"
)

func TestFromFen(t *testing.T) {
	type args struct {
		c string
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		// TODO: Add test cases.
		{"kolla 1", args{"p"}, 1},
		{"kolla 2", args{"P"}, 0},
		{"kolla 3", args{"q"}, 9},
		{"kolla 4", args{"Q"}, 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FromFen(tt.args.c); got != tt.want {
				t.Errorf("FromFen() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestScore(t *testing.T) {
	const MG = 0 // Middle game
	const EG = 1 // End game

	type args struct {
		pc    int
		stage int
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{"", args{Pawn, MG}, 85},
		{"", args{Pawn, EG}, 95},
		{"", args{Knight, MG}, 325},
		{"", args{Knight, EG}, 325},
		{"", args{Bishop, MG}, 325},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Score(tt.args.pc, tt.args.stage); got != tt.want {
				t.Errorf("Score() = %v, want %v", got, tt.want)
			}
		})
	}
}
