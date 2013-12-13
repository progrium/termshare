# termshare

Quick and easy terminal sharing. 

Share interactive control with a copilot and/or a readonly view of your terminal with others. Copilots and viewers can use the client or a web-based terminal.

NOTICE: This tool can already be quite dangerous, but it's also currently NOT using TLS. When the [termshare Gittip](https://www.gittip.com/termshare) reaches $5/wk, we'll run the service with SSL.

```
$ termshare 
 _                          _                    
| |_ ___ _ __ _ __ ___  ___| |__   __ _ _ __ ___ 
| __/ _ \ '__| '_ ` _ \/ __| '_ \ / _` | '__/ _ \
| ||  __/ |  | | | | | \__ \ | | | (_| | | |  __/
 \__\___|_|  |_| |_| |_|___/_| |_|\__,_|_|  \___|

Session URL: http://termsha.re/3b5cc0d7-185f-4568-6e2d-7d6e77f836aa


[termshare] $
```

## Install

```
curl -sL https://github.com/progrium/termshare/releases/download/v0.1.0/termshare_v0.1.0_$(uname -s)_$(uname -m).tgz | tar -C /usr/local/bin -zxf -
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
