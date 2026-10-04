package main

import (
	"fmt"
	specqr "github.com/SpecQR/SpecQR-Go"
	"os"
)

func main() {
	q, err := specqr.Generate("Hello, 日本語", specqr.DefaultOptions())
	if err != nil {
		panic(err)
	}
	png, err := q.ToPNG()
	if err != nil {
		panic(err)
	}
	if err = os.WriteFile("hello.png", png, 0644); err != nil {
		panic(err)
	}
	fmt.Printf("version=%d size=%d mask=%d\n", q.Version(), q.Size(), q.Mask())
}
