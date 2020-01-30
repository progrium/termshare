package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"nhooyr.io/websocket"
	"github.com/heroku/hk/term"
	"github.com/creack/pty"
	"github.com/google/uuid"
)

const VERSION = "v0.3.0"
const msgType = websocket.MessageText

var daemon *bool = flag.Bool("d", false, "run the server daemon")
var copilot *bool = flag.Bool("c", false, "allow a copilot to join to share control")
var private *bool = flag.Bool("p", false, "only allow a copilot and no viewers")
var server *string = flag.String("s", "termsha.re:443", "use a different server to start session")
var notls *bool = flag.Bool("n", false, "do not use tls endpoints")
var version *bool = flag.Bool("v", false, "print version and exit")

var banner = ` _                          _                    
| |_ ___ _ __ _ __ ___  ___| |__   __ _ _ __ ___ 
| __/ _ \ '__| '_ ` + "`" + ` _ \/ __| '_ \ / _` + "`" + ` | '__/ _ \
| ||  __/ |  | | | | | \__ \ | | | (_| | | |  __/
 \__\___|_|  |_| |_| |_|___/_| |_|\__,_|_|  \___|

Running this open source service supported 100% by community.
Donate: https://www.gittip.com/termshare

Session URL: {{URL}}
`

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage:  %v [session-url]\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Starts termshare sesion or connects to session if session-url is specified\n")
		flag.PrintDefaults()
	}
}

type session struct {
	Name          string
	AllowCopilot  bool
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

func (s sessions) Create(name string, copilot, private bool) (*session, error) {
	if sess, _ := s.Get(name); sess != nil {
		return nil, errors.New("session already exists")
	}
	sess := &session{
		Name:          name,
		AllowCopilot:  copilot,
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
		if err != nil || n != len(data) {
			delete(v.v, w)
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
	w net.Conn
	b *bytes.Buffer
}

func (bw *bufferWriter) Write(p []byte) (n int, err error) {
	//defer func() {
	//	log.Printf("bufferWriter.Write() n(%d) err(%v) %s", n, err, p[:n])
	//}()

	if bw.b == nil {
		bw.b = new(bytes.Buffer)
	}
	if bw.b.Len() > 0 && bw.w != nil {
		if _, err = bw.b.WriteTo(bw.w); err != nil {
			bw.w = nil
			err = nil
		}
		if bw.w != nil {
			_ = bw.w.Close()
		}
	}
	if bw.w != nil {
		if n, err = bw.w.Write(p); err != nil {
			bw.w = nil
			return bw.b.Write(p)
		}
		return n, err
	} else {
		return bw.b.Write(p)
	}
}

func readResponse(resp *http.Response) (string, error) {
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == 200 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		return string(body), nil
	} else {
		return "", errors.New("unexpected status: " + string(resp.StatusCode))
	}
}

func baseUrl(protocol string) string {
	if !*notls {
		protocol = protocol + "s"
	}
	return protocol + "://" + *server
}

func createSession() {
	ctx := context.Background()

	name, err := uuid.NewRandom()
	if err != nil {
		panic(err)
	}
	values := map[bool]string{
		true:  "true",
		false: "",
	}
	resp, err := http.PostForm(baseUrl("http")+"/"+name.String(),
		url.Values{"copilot": {values[*copilot]}, "private": {values[*private]}})
	if err != nil {
		panic(err)
	}
	body, err := readResponse(resp)
	if err != nil {
		log.Fatal("unable to open session")
	}
	fmt.Println(body)

	wsConn, _, err := websocket.Dial(ctx, baseUrl("ws")+"/"+name.String(), nil)
	if err != nil {
		panic(err)
	}
	conn := websocket.NetConn(ctx, wsConn, msgType)

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
	ptyF, err := pty.Start(cmd)
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
		io.Copy(io.MultiWriter(os.Stdout, conn), ptyF)
		eof <- true
	}()
	go func() {
		io.Copy(ptyF, os.Stdin)
		eof <- true
	}()
	go func() {
		Copy(ptyF, conn)
		eof <- true
	}()
	go func() {
		for {
			if _, err := conn.Write([]byte("\x00")); err != nil {
				return
			}
			time.Sleep(10 * time.Second)
		}
	}()
	<-eof
}

// Copy copies from src to dst until either EOF is reached
// on src or an error occurs. It returns the number of bytes
// copied and the first error encountered while copying, if any.
//
// A successful Copy returns err == nil, not err == EOF.
// Because Copy is defined to read from src until EOF, it does
// not treat an EOF from Read as an error to be reported.
//
// If src implements the WriterTo interface,
// the copy is implemented by calling src.WriteTo(dst).
// Otherwise, if dst implements the ReaderFrom interface,
// the copy is implemented by calling dst.ReadFrom(src).
func Copy(dst io.Writer, src io.Reader) (written int64, err error) {
	return copyBuffer(dst, src, nil)
}

// copyBuffer is the actual implementation of Copy and CopyBuffer.
// if buf is nil, one is allocated.
func copyBuffer(dst io.Writer, src io.Reader, buf []byte) (written int64, err error) {
	log.Printf("copyBuffer: start...")
	// If the reader has a WriteTo method, use it to do the copy.
	// Avoids an allocation and a copy.
	if wt, ok := src.(io.WriterTo); ok {
		return wt.WriteTo(dst)
	}
	// Similarly, if the writer has a ReadFrom method, use it to do the copy.
	if rt, ok := dst.(io.ReaderFrom); ok {
		return rt.ReadFrom(src)
	}
	if buf == nil {
		size := 10// 32 * 1024
		if l, ok := src.(*io.LimitedReader); ok && int64(size) > l.N {
			if l.N < 1 {
				size = 1
			} else {
				size = int(l.N)
			}
		}
		buf = make([]byte, size)
	}
	for {
		log.Printf("copyBuffer: start read...")
		nr, er := src.Read(buf)
		log.Printf("copyBuffer(src.Read) n(%d) err(%v) %s", nr, er, buf[0:nr])
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			log.Printf("copyBuffer(dst.Write) n(%d) err(%v) %s", nw, ew, buf[0:nw])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	return written, err
}

func joinSession(sessionUrl string) {
	ctx := context.Background()

	pUrl, err := url.Parse(sessionUrl)
	if err != nil {
		log.Fatal(err)
	}
	if !strings.Contains(pUrl.Host, ":") {
		if *notls {
			*server = pUrl.Host + ":80"
		} else {
			*server = pUrl.Host + ":443"
		}
	} else {
		*server = pUrl.Host
	}
	wsConn, _, err := websocket.Dial(ctx, baseUrl("ws")+pUrl.Path, nil)
	if err != nil {
		log.Fatal(err)
	}
	conn := websocket.NetConn(ctx, wsConn, msgType)

	if err := term.MakeRaw(os.Stdin); err != nil {
		log.Fatal(err)
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

func startDaemon() {
	sessions := sessions{s: make(map[string]*session)}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.RequestURI == "/":
			http.Redirect(w, r, "https://github.com/progrium/termshare", 301)
		case r.RequestURI == "/favicon.ico":
			return
		case r.RequestURI == "/version":
			w.Write([]byte(VERSION))
		case strings.HasPrefix(r.RequestURI, "/download/"):
			parts := strings.Split(r.RequestURI, "/")
			os := parts[len(parts)-1]
			http.Redirect(w, r, "https://github.com/progrium/termshare/releases/download/"+VERSION+"/termshare_"+VERSION+"_"+os+"_x86_64.tgz", 301)
		default:
			parts := strings.Split(r.RequestURI, "/")
			sessionName := parts[1]

			if r.Method == "POST" {
				r.ParseForm()
				_, err := sessions.Create(sessionName, r.Form.Get("copilot") != "", r.Form.Get("private") != "")
				if err != nil {
					log.Println(err)
					w.WriteHeader(http.StatusConflict)
					return
				}
				logline := sessionName + ": session created"
				if r.Form.Get("copilot") != "" {
					logline = logline + " [copilot]"
				}
				if r.Form.Get("private") != "" {
					logline = logline + " [private]"
				}
				log.Println(logline)
				baseUrl := baseUrl("http")
				baseUrl = strings.TrimSuffix(baseUrl, ":443")
				baseUrl = strings.TrimSuffix(baseUrl, ":80")
				w.Write([]byte(strings.Replace(banner, "{{URL}}", baseUrl+"/"+sessionName, 1)))
				return
			}

			session, err := sessions.Get(sessionName)
			if err != nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			isWebsocket := r.Header.Get("Upgrade") == "websocket"
			switch {
			case session.Pilot == nil && isWebsocket:
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					wsConn, err := websocket.Accept(w, r, nil)
					if err != nil {
						return
					}
					conn := websocket.NetConn(r.Context(), wsConn, msgType)

					session.Pilot = conn
					log.Println(sessionName + ": pilot connected")
					if _, err := io.Copy(io.MultiWriter(session.Viewers, session.CopilotBuffer), session.Pilot); err != nil {
						if err == io.EOF {
							close(session.EOF)
						} else {
							log.Println("pilot writing error: ", err)
						}
					}
				}).ServeHTTP(w, r)

			case session.Pilot != nil && session.Copilot == nil && session.AllowCopilot && isWebsocket:
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					wsConn, err := websocket.Accept(w, r, nil)
					if err != nil {
						return
					}
					conn := websocket.NetConn(r.Context(), wsConn, msgType)

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
				if isWebsocket {
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						wsConn, err := websocket.Accept(w, r, nil)
						if err != nil {
							return
						}
						conn := websocket.NetConn(r.Context(), wsConn, msgType)

						session.Viewers.Add(conn)
						log.Println(sessionName + ": viewer connected [websocket]")
						<-session.EOF
					}).ServeHTTP(w, r)
				} else {
					if strings.HasPrefix(r.Header.Get("User-Agent"), "curl/") {
						session.Viewers.Add(FlushWriter(w))
						log.Println(sessionName + ": viewer connected [http]")
						<-session.EOF
					} else {
						log.Println(sessionName + ": viewer connected [browser]")
						w.Write(term_html())
					}
				}
			}
		}
	})
	port := ":" + os.Getenv("PORT")
	log.Println("Termshare server started on " + port + "...")
	log.Fatal(http.ListenAndServe(port, nil))
}

func main() {
	flag.Parse()

	if *version {
		fmt.Println(VERSION)
		return
	}

	if *daemon {
		startDaemon()
	} else {
		if flag.Arg(0) == "" {
			createSession()
		} else {
			joinSession(flag.Arg(0))
		}
	}
}
