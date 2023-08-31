
sprinkler: *.go cmd/module/*.go *.html
	go build -o sprinkler cmd/module/cmd.go

test:
	go test

lint:
	gofmt -w -s .
