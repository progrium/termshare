package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"code.google.com/p/go.net/websocket"
	"github.com/heroku/hk/term"
	"github.com/kr/pty"
	"github.com/nu7hatch/gouuid"
)

var daemon *bool = flag.Bool("d", false, "run the server daemon")
var readonly *bool = flag.Bool("r", false, "only allow participants viewing capability")
var server *string = flag.String("s", "young-dusk-7491.herokuapp.com:80", "use a different server")

type session struct {
	name         string
	readonly     bool
	participants []io.ReadWriteCloser
	presenterW   io.WriteCloser
	presenterR   io.ReadCloser
	participantW io.WriteCloser
	participantR io.ReadCloser
}

func present() {
	name, err := uuid.NewV4()
	if err != nil {
		panic(err)
	}
	resp, err := http.PostForm("http://"+*server+"/sessions", url.Values{"name": {name.String()}})
	if err != nil {
		panic(err)
	}
	var body []byte
	if resp.StatusCode == 200 {
		body, err = ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			panic(err)
		}
	} else {
		resp.Body.Close()
		log.Fatal("unable to open session")
	}
	fmt.Println(string(body))

	conn, err := websocket.Dial("ws://"+*server+"/"+name.String()+"/presenter", "", "http://"+*server)
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
	exitSignal := make(chan os.Signal)
	signal.Notify(exitSignal, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-exitSignal
		term.Restore(os.Stdin)
		os.Exit(0)
	}()
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
	go func() {
		for {
			_, err := conn.Write([]byte("\x00"))
			if err != nil {
				return
			}
			time.Sleep(10 * time.Second)
		}
	}()
	<-eof
}

func participate(name string) {
	conn, err := websocket.Dial("ws://"+*server+"/"+name, "", "http://"+*server)
	if err != nil {
		panic(err)
	}
	if err := term.MakeRaw(os.Stdin); err != nil {
		panic(err)
	}
	exitSignal := make(chan os.Signal)
	signal.Notify(exitSignal, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-exitSignal
		term.Restore(os.Stdin)
		os.Exit(0)
	}()
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

	// TODO: lock
	sessions := make(map[string]session)

	if *daemon {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.RequestURI == "/":
				http.Redirect(w, r, "http://progrium.viewdocs.io/termshare", 301)
			case r.RequestURI == "/favicon.ico":
				return
			case r.RequestURI == "/sessions" && r.Method == "POST":
				r.ParseForm()
				name := r.PostForm.Get("name")
				_, found := sessions[name]
				if found {
					w.WriteHeader(http.StatusConflict)
					return
				}
				s := session{
					name:     name,
					readonly: false,
				}
				s.presenterR, s.presenterW = io.Pipe()
				s.participantR, s.participantW = io.Pipe()
				sessions[name] = s
				log.Println(name + ": session created")
				w.Write([]byte("http://termsha.re/" + name + "\n"))
			case strings.HasSuffix(r.RequestURI, "/presenter"):
				parts := strings.Split(r.RequestURI, "/")
				name := parts[1]
				s, found := sessions[name]
				if !found {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				if s.presenterW == nil {
					w.WriteHeader(http.StatusConflict)
					return
				}
				websocket.Handler(func(ws *websocket.Conn) {
					eof := make(chan bool, 1)
					go func() {
						io.Copy(s.presenterW, ws)
						eof <- true
					}()
					go func() {
						io.Copy(ws, s.participantR)
						eof <- true
					}()
					log.Println(name + ": presenter connected")
					<-eof
					delete(sessions, name)
				}).ServeHTTP(w, r)
			default:
				parts := strings.Split(r.RequestURI, "/")
				name := parts[1]
				s, found := sessions[name]
				if !found {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				if s.presenterW == nil {
					w.WriteHeader(http.StatusConflict)
					return
				}
				websocket.Handler(func(ws *websocket.Conn) {
					eof := make(chan bool, 1)
					go func() {
						io.Copy(s.participantW, ws)
						eof <- true
					}()
					go func() {
						io.Copy(ws, s.presenterR)
						eof <- true
					}()
					s.participantW.Write([]byte("\x07"))
					log.Println(name + ": participant connected")
					<-eof
				}).ServeHTTP(w, r)
			}
		})
		log.Println("Termshare server started...")
		log.Fatal(http.ListenAndServe(":"+os.Getenv("PORT"), nil))
	} else {
		if flag.Arg(0) == "" {
			present()
		} else {
			participate(flag.Arg(0))
		}
	}
}
