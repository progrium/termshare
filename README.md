# termshare

Quick and easy terminal sharing for getting quick help or pair sysadmin'ing. 

Share interactive control with a copilot and/or a readonly view of your terminal with others. Copilots and viewers can use the client or a web-based terminal.

The service is run by the support of the community via Gittip donations. [Please donate to keep termshare running and support the work of the author.](https://www.gittip.com/termshare/)

```
$ termshare 
 _                          _                    
| |_ ___ _ __ _ __ ___  ___| |__   __ _ _ __ ___ 
| __/ _ \ '__| '_ ` _ \/ __| '_ \ / _` | '__/ _ \
| ||  __/ |  | | | | | \__ \ | | | (_| | | |  __/
 \__\___|_|  |_| |_| |_|___/_| |_|\__,_|_|  \___|

Session URL: https://termsha.re/3b5cc0d7-185f-4568-6e2d-7d6e77f836aa

[termshare] $
```

## Install

```
curl -sL https://termsha.re/download/$(uname -s) | tar -C /usr/local/bin -zxf -
```

## Usage

```
Usage:  termshare [session-url]

Starts termshare sesion or connects to session if session-url is specified

  -c=false: allow a copilot to join to share control
  -d=false: run the server daemon
  -n=false: do not use tls endpoints
  -p=false: only allow a copilot and no viewers
  -s="termsha.re:443": use a different server to start session
  -v=false: print version and exit
```

### License

BSD
