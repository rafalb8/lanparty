BINARY := lanparty

.PHONY: build clean

build:
	CGO_ENABLED=0 go build -o $(BINARY) .
	sudo setcap cap_net_admin+ep $(BINARY)

clean:
	rm -f $(BINARY)
