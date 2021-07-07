package main

import "os"

func cancelSigs() []os.Signal {
	return []os.Signal{os.Interrupt}
}
