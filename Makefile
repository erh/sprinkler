
sprinkler: *.go cmd/module/*.go *.html
	-mkdir bin
	go build -o bin/sprinkler cmd/module/cmd.go

test:
	go test

lint:
	gofmt -w -s .

updaterdk:
	go get go.viam.com/rdk@latest
	go mod tidy

