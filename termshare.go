package main

import (
	"flag"
	"io"
	"net"
	"os"
	"os/exec"

	"github.com/dotcloud/docker/term"
	"github.com/kr/pty"
)

var daemon *bool = flag.Bool("d", false, "run server")

var presenterReader, presenterWriter = io.Pipe()
var participantReader, participantWriter = io.Pipe()

func runParticipantServer() {
	listener, err := net.Listen("tcp", ":9000")
	if err != nil {
		panic(err)
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			panic(err)
		}
		go handleParticipantServer(conn)
	}
}

func runPresenterServer() {
	listener, err := net.Listen("tcp", ":8000")
	if err != nil {
		panic(err)
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			panic(err)
		}
		go handlePresenterServer(conn)
	}
}

func handleParticipantServer(conn net.Conn) {
	eof := make(chan bool, 1)
	go func() {
		io.Copy(participantWriter, conn)
		eof <- true
	}()
	go func() {
		io.Copy(conn, presenterReader)
		eof <- true
	}()
	<-eof
}

func handlePresenterServer(conn net.Conn) {
	eof := make(chan bool, 1)
	go func() {
		io.Copy(presenterWriter, conn)
		eof <- true
	}()
	go func() {
		io.Copy(conn, participantReader)
		eof <- true
	}()
	<-eof
}

func runPresenter() {
	conn, err := net.Dial("tcp", "127.0.0.1:8000")
	if err != nil {
		panic(err)
	}
	cmd := exec.Command(os.Getenv("SHELL"))
	pty, err := pty.Start(cmd)
	if err != nil {
		panic(err)
	}
	terminalFd := os.Stdin.Fd()
	oldState, err := term.SetRawTerminal(terminalFd)
	if err != nil {
		panic(err)
	}
	defer term.RestoreTerminal(terminalFd, oldState)
	eof := make(chan bool, 1)
	go func() {
		io.Copy(io.MultiWriter(os.Stdout, conn), pty)
		eof <- true
	}()
	go func() {
		io.Copy(pty, os.Stdin)
		eof <- true
	}()
	go func() {
		io.Copy(pty, conn)
		eof <- true
	}()
	<-eof
}

func runParticipant(port string) {
	conn, err := net.Dial("tcp", "127.0.0.1:"+port)
	if err != nil {
		panic(err)
	}
	terminalFd := os.Stdin.Fd()
	oldState, err := term.SetRawTerminal(terminalFd)
	if err != nil {
		panic(err)
	}
	defer term.RestoreTerminal(terminalFd, oldState)
	eof := make(chan bool, 1)
	go func() {
		io.Copy(os.Stdout, conn)
		eof <- true
	}()
	go func() {
		io.Copy(conn, os.Stdin)
		eof <- true
	}()
	<-eof
}

func main() {
	flag.Parse()
	if *daemon {
		go runPresenterServer()
		runParticipantServer()
	} else {
		if flag.Arg(0) == "" {
			runPresenter()
		} else {
			runParticipant(flag.Arg(0))
		}
	}
}
