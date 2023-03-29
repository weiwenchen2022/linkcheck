package main

import (
	"fmt"
	"log"
	"os"

	"linkcheck"
)

func init() {
	log.SetFlags(0)
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage %s url\n", os.Args[0])
		os.Exit(1)
	}

	l := linkcheck.NewLinkChecker()
	l.Main(os.Args[1])
}
