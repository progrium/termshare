package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"code.google.com/p/go.net/websocket"
	"github.com/heroku/hk/term"
	"github.com/kr/pty"
	"github.com/nu7hatch/gouuid"
)

var daemon *bool = flag.Bool("d", false, "run the server daemon")
var broadcast *bool = flag.Bool("b", false, "only allow readonly viewers and no copilot")
var private *bool = flag.Bool("p", false, "only allow a copilot and no viewers")
var server *string = flag.String("s", "young-dusk-7491.herokuapp.com:80", "use a different server")

type session struct {
	Name          string
	Broadcast     bool
	Private       bool
	Viewers       *viewers
	Pilot         io.ReadWriteCloser
	Copilot       io.ReadWriteCloser
	CopilotBuffer *bufferWriter
	EOF           chan struct{}
}

type sessions struct {
	sync.Mutex
	s map[string]*session
}

func (s sessions) Get(name string) (sess *session, err error) {
	s.Lock()
	defer s.Unlock()
	sess, found := s.s[name]
	if !found {
		err = errors.New("session not found")
		return
	}
	return
}

func (s sessions) Create(name string, broadcast, private bool) (*session, error) {
	if sess, _ := s.Get(name); sess != nil {
		return nil, errors.New("session already exists")
	}
	sess := &session{
		Name:          name,
		Broadcast:     broadcast,
		Private:       private,
		Viewers:       &viewers{v: make(map[io.Writer]struct{})},
		EOF:           make(chan struct{}),
		CopilotBuffer: &bufferWriter{},
	}
	s.Lock()
	defer s.Unlock()
	s.s[name] = sess
	return sess, nil
}

func (s sessions) Delete(name string) {
	s.Lock()
	defer s.Unlock()
	delete(s.s, name)
}

type viewers struct {
	sync.Mutex
	v map[io.Writer]struct{}
}

func (v viewers) Write(data []byte) (n int, err error) {
	v.Lock()
	defer v.Unlock()
	for w := range v.v {
		n, err = w.Write(data)
		if err != nil {
			delete(v.v, w)
		}
		if n != len(data) {
			err = io.ErrShortWrite
			return
		}
	}
	return len(data), nil
}

func (v viewers) Add(viewer io.Writer) {
	v.Lock()
	defer v.Unlock()
	v.v[viewer] = struct{}{}
}

type flushWriter struct {
	f http.Flusher
	w io.Writer
}

func (fw flushWriter) Write(p []byte) (n int, err error) {
	n, err = fw.w.Write(p)
	if fw.f != nil {
		fw.f.Flush()
	}
	return
}

func FlushWriter(writer io.Writer) flushWriter {
	fw := flushWriter{w: writer}
	if f, ok := writer.(http.Flusher); ok {
		fw.f = f
	}
	return fw
}

type bufferWriter struct {
	w *websocket.Conn
	b *bytes.Buffer
}

func (bw *bufferWriter) Write(p []byte) (n int, err error) {
	if bw.b == nil {
		bw.b = new(bytes.Buffer)
	}
	if bw.b.Len() > 0 && bw.w != nil {
		if _, err = bw.b.WriteTo(bw.w); err != nil {
			bw.w = nil
			err = nil
		}
	}
	if bw.w != nil {
		n, err = bw.w.Write(p)
		if err != nil {
			bw.w = nil
			return bw.b.Write(p)
		}
		return n, err
	} else {
		return bw.b.Write(p)
	}
}

func share() {
	name, err := uuid.NewV4()
	if err != nil {
		panic(err)
	}
	resp, err := http.Post("http://"+*server+"/"+name.String(), "application/x-www-form-urlencoded", strings.NewReader(""))
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

	conn, err := websocket.Dial("ws://"+*server+"/"+name.String(), "", "http://"+*server)
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

func connect(sessionName string) {
	conn, err := websocket.Dial("ws://"+*server+"/"+sessionName, "", "http://"+*server)
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

func sessionNameFromRequest(r *http.Request) string {
	parts := strings.Split(r.RequestURI, "/")
	return parts[1]
}

func isWebsocketRequest(r *http.Request) bool {
	return r.Header.Get("Upgrade") == "websocket"
}

func main() {
	flag.Parse()

	if *daemon {
		sessions := sessions{s: make(map[string]*session)}

		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.RequestURI == "/":
				http.Redirect(w, r, "http://progrium.viewdocs.io/termshare", 301)
			case r.RequestURI == "/favicon.ico":
				return
			default:
				sessionName := sessionNameFromRequest(r)
				session, err := sessions.Get(sessionName)
				if r.Method == "POST" {
					_, err = sessions.Create(sessionName, false, false)
					if err != nil {
						log.Println(err)
						w.WriteHeader(http.StatusConflict)
						return
					}
					log.Println(sessionName + ": session created")
					w.Write([]byte("http://termsha.re/" + sessionName + "\n"))
					return
				}
				if err != nil {
					//w.WriteHeader(http.StatusNotFound)
					return
				}
				switch {
				case session.Pilot == nil && isWebsocketRequest(r):
					websocket.Handler(func(conn *websocket.Conn) {
						session.Pilot = conn
						log.Println(sessionName + ": pilot connected")
						_, err := io.Copy(io.MultiWriter(session.Viewers, session.CopilotBuffer), session.Pilot)
						if err == io.EOF {
							close(session.EOF)
						} else {
							log.Println("pilot writing error: ", err)
						}
					}).ServeHTTP(w, r)
				case session.Pilot != nil && session.Copilot == nil && !session.Broadcast && isWebsocketRequest(r):
					websocket.Handler(func(conn *websocket.Conn) {
						session.Copilot = conn
						session.CopilotBuffer.w = conn
						session.Pilot.Write([]byte("\x07")) // ding!
						log.Println(sessionName + ": copilot connected")
						eof := make(chan struct{})
						go func() {
							io.Copy(session.Pilot, session.Copilot)
							session.Copilot = nil
							session.CopilotBuffer.w = nil
							eof <- struct{}{}
						}()
						<-eof
					}).ServeHTTP(w, r)
				case session.Pilot != nil && !session.Private:
					if isWebsocketRequest(r) {
						websocket.Handler(func(conn *websocket.Conn) {
							session.Viewers.Add(conn)
							log.Println(sessionName + ": viewer connected (websocket)")
							<-session.EOF
						}).ServeHTTP(w, r)
					} else {
						// TODO: check for curl, otherwise serve static page with term.js
						session.Viewers.Add(FlushWriter(w))
						log.Println(sessionName + ": viewer connected (http stream)")
						<-session.EOF
					}
				}
			}
		})
		log.Println("Termshare server started...")
		log.Fatal(http.ListenAndServe(":"+os.Getenv("PORT"), nil))
	} else {
		if flag.Arg(0) == "" {
			share()
		} else {
			connect(flag.Arg(0))
		}
	}
}
