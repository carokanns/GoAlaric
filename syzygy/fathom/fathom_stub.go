//go:build !cgo

package fathom

func backendCompiled() bool { return false }

func backendInit(string) (bool, int) { return false, 0 }

func backendFree() {}

func backendProbeWDL(Position) (int, bool) { return 0, false }

func backendProbeRoot(Position) (RootResult, bool) { return RootResult{}, false }
