VERSION=$(shell ./termshare -v)
HARDWARE=$(shell uname -m)

build:
	go build

release: build
	mkdir release
	GOOS=linux go build -o release/termshare.linux
	cd release && tar -zcf termshare_$(VERSION)_Linux_$(HARDWARE).tgz termshare.linux
	GOOS=darwin go build -o release/termshare.darwin
	cd release && tar -zcf termshare_$(VERSION)_Darwin_$(HARDWARE).tgz termshare.darwin

clean:
	rm -rf release