set quiet

binary := "switchboard"

[private]
default:
    @just --list

[doc("Build the binary")]
build:
    go build -o {{binary}} .

[doc("Format code (goimports + gofumpt)")]
fmt:
    go tool goimports -w .
    go tool gofumpt -w .

[doc("Run go vet")]
vet:
    go vet ./...

[doc("Run the test suite")]
test:
    go test ./...

[doc("Vet + test")]
check: vet test

[doc("Install to GOBIN / $GOPATH/bin")]
install:
    go install .

[doc("Run without building")]
run *args:
    go run . {{args}}

[doc("Try the TUI: examples/demo.json (items) + examples/switchboard.yaml (switch commands)")]
demo:
    go run . --config examples/switchboard.yaml examples/demo.json
