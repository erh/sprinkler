
bin/sprinkler: *.go cmd/module/*.go *.html *.mod
	-mkdir bin
	go build -o bin/sprinkler cmd/module/cmd.go

test:
	go test

lint:
	gofmt -w -s .

updaterdk:
	go get go.viam.com/rdk@latest
	go mod tidy

module: bin/sprinkler
	tar czf module.tar.gz bin/sprinkler
