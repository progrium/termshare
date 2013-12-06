package main

import (
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"

	"code.google.com/p/go.net/websocket"
	"github.com/heroku/hk/term"
	"github.com/kr/pty"
)

var daemon *bool = flag.Bool("d", false, "run server")
var presenter *bool = flag.Bool("p", false, "run as presenter")

var presenterReader, presenterWriter = io.Pipe()
var participantReader, participantWriter = io.Pipe()

func participantHandler(ws *websocket.Conn) {
	eof := make(chan bool, 1)
	go func() {
		io.Copy(participantWriter, ws)
		eof <- true
	}()
	go func() {
		io.Copy(ws, presenterReader)
		eof <- true
	}()
	<-eof
}

func presenterHandler(ws *websocket.Conn) {
	eof := make(chan bool, 1)
	go func() {
		io.Copy(presenterWriter, ws)
		eof <- true
	}()
	go func() {
		io.Copy(ws, participantReader)
		eof <- true
	}()
	<-eof
}

func runPresenter(host string) {
	conn, err := websocket.Dial("ws://"+host+"/presenter", "", "http://"+host)
	if err != nil {
		panic(err)
	}
	cols, err := term.Cols()
	if err != nil {
		panic(err)
	}
	lines, err := term.Lines()
	if err != nil {
		panic(err)
	}
	cmd := exec.Command(os.Getenv("SHELL"))
	cmd.Env = []string{
		"PS1=[termshare] \\W$ ",
		"TERM=" + os.Getenv("TERM"),
		"HOME=" + os.Getenv("HOME"),
		"USER=" + os.Getenv("USER"),
		"COLUMNS=" + strconv.Itoa(cols),
		"LINES=" + strconv.Itoa(lines),
	}
	pty, err := pty.Start(cmd)
	if err != nil {
		panic(err)
	}
	if err := term.MakeRaw(os.Stdin); err != nil {
		panic(err)
	}
	defer term.Restore(os.Stdin)
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

func runParticipant(host string) {
	conn, err := websocket.Dial("ws://"+host+"/participant", "", "http://"+host)
	if err != nil {
		panic(err)
	}
	if err := term.MakeRaw(os.Stdin); err != nil {
		panic(err)
	}
	defer term.Restore(os.Stdin)
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
		http.Handle("/presenter", websocket.Handler(presenterHandler))
		http.Handle("/participant", websocket.Handler(participantHandler))
		log.Fatal(http.ListenAndServe(":"+os.Getenv("PORT"), nil))
	} else {
		if *presenter {
			runPresenter(flag.Arg(0))
		} else {
			runParticipant(flag.Arg(0))
		}
	}
}
