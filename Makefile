VERSION=$(shell ./termshare -v)
HARDWARE=$(shell uname -m)

build:
	go build

release: build
	mkdir release
	GOOS=linux go build -o release/termshare
	cd release && tar -zcf termshare_$(VERSION)_Linux_$(HARDWARE).tgz termshare
	GOOS=darwin go build -o release/termshare
	cd release && tar -zcf termshare_$(VERSION)_Darwin_$(HARDWARE).tgz termshare
	rm release/termshare

clean:
	rm -rf release