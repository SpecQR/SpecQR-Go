package specqr_test

import (
	"fmt"
	specqr "github.com/SpecQR/SpecQR-Go"
)

func ExampleGenerate() {
	q, err := specqr.Generate("HELLO WORLD", specqr.DefaultOptions())
	if err != nil {
		panic(err)
	}
	fmt.Println(q.Version(), q.Size(), q.ECC())
	// Output: 1 21 M
}
func ExampleGenerateBytes() {
	q, err := specqr.GenerateBytes([]byte{0, 255, 128}, specqr.Options{})
	if err != nil {
		panic(err)
	}
	fmt.Println(q.Segments()[0].Count())
	// Output: 3
}
