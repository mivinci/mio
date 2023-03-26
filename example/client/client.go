package main

import (
	"context"
	"fmt"
	"net"

	"github.com/mivinci/mio"
)

func main() {
	conn, err := net.Dial("tcp", ":8080")
	if err != nil {
		panic(err)
	}

	sess := mio.New(conn)
	str, err := sess.Open(context.Background())
	if err != nil {
		panic(err)
	}

	b := []byte("Aa")
	_, err = str.Write(b)
	if err != nil {
		panic(err)
	}

	p := make([]byte, 32)
	n, err := str.Read(p)
	if err != nil {
		panic(err)
	}
	fmt.Println(n, string(p[:n]))
}
