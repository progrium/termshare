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

## Running the Termshare Server Locally

The Termshare server is meant to be run on Heroku, but you can also run it locally. There are just a few tricky bits. Run the server like this:

	$ PORT=8080 termshare -d -n -s localhost:8080

Now when creating a session, you not only need to specify to use the local server, but you need to pass `-n` otherwise it will try to connect with TLS, which is only available via Heroku.

	$ termshare -n -s localhost:8080

The Session URL it gives you should be accurate, but if you use it with termshare you do still need to pass `-n`. For example:

	$ termshare -n http://localhost:8080/43aa4bd7-6583-41aa-446d-dc32fcceba2e

### License

BSD
