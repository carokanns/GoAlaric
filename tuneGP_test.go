//go:build tune
// +build tune

package main

import (
	"strings"
	"testing"
)

func Test_mirror(t *testing.T) {

	tests := []struct {
		name string
		epd  string
		want string
	}{
		{"", "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - c0 alaric1-alaric2 TuneGames Ingared 2018.07.31; c1 1-0;", "x"},
		{"", "r2qkbnr/ppp1pppp/2n5/3P4/2p1P1b1/5N2/PP3PPP/RNBQKB1R b KQkq - c0 alaric2-alaric1 TuneGames Ingared 2018.07.31; c1 1-0;", "w"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mirror(tt.epd)
			x := strings.Fields(got)[1]

			if x != tt.want {
				t.Errorf("mirror() = %v, want %v", got, tt.want)
			}
		})
	}
}
