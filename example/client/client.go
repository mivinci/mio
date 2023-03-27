package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/mivinci/mio"
)

func main() {
	conn, err := net.Dial("tcp", ":8080")
	if err != nil {
		log.Fatal(err)
	}

	var wg sync.WaitGroup

	C := 1000
	N := 100
	sess := mio.New(conn)

	ch := make(chan struct{}, C*N)

	wg.Add(C)

	for i := 0; i < C; i++ {
		go func(i int) {
			defer wg.Done()
			str, err := sess.Open(context.Background())
			if err != nil {
				log.Fatal(err)
			}

			for j := 0; j < N; j++ {
				b := []byte(fmt.Sprintf("Aa %d-%d", i, j))
				_, err = str.Write(b)
				if err != nil {
					log.Fatal(err)
				}

				p := make([]byte, 32)
				n, err := str.Read(p)
				if err != nil {
					log.Fatal(err)
				}
				p = p[:n]
				if !bytes.Equal(b, p) {
					log.Fatalf("unequal %d-%d", i, j)
				}
				ch <- struct{}{}
			}
		}(i)
	}

	wg.Wait()

	n := C * N
	if len(ch) != n {
		log.Fatalf("want %d but got %d", n, len(ch))
	}
}
