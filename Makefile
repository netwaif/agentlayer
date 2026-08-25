.PHONY: build test vet install clean

build:
	go build -o agentlayer .

test:
	go test ./... -count=1

vet:
	go vet ./...

# 로컬 설치 — hook이 PATH에서 agentlayer를 찾을 수 있어야 한다
install: build
	install -m 0755 agentlayer $(HOME)/.local/bin/agentlayer

clean:
	rm -f agentlayer
