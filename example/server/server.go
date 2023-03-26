package main

import (
	"io"
	"net"

	"github.com/mivinci/mio"
)

func main() {
	l, err := net.Listen("tcp", ":8080")
	if err != nil {
		panic(err)
	}

	conn, err := l.Accept()
	if err != nil {
		panic(err)
	}

	sess := mio.New(conn)
	for {
		str, err := sess.Accept()
		if err != nil {
			panic(err)
		}

		go handle(str)
	}
}

func handle(str mio.Stream) {
	io.Copy(str, str)
	str.Close()
}
