# termshare

Quick and easy terminal sharing. 

Share interactive control with a copilot and/or a readonly view of your terminal with others. Copilots and viewers can use the client or a web-based terminal.

## Install

```
curl -sL http://termsha.re/download | tar -C /usr/local/bin -zxf -
```

## Usage

```
Usage:  termshare [session-url]

Starts termshare sesion or connects to session if session-url is specified

  -b=false: only allow readonly viewers and no copilot
  -d=false: run the server daemon
  -p=false: only allow a copilot and no viewers
  -s="termsha.re:80": use a different server to start session
  -v=false: print version and exit
```