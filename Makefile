build:
	rm -rf bin
	GOOS=linux GOARCH=amd64 go build -o bin/generate main.go
	cp -r cloudinit bin/
	cp -r config bin/
	cp config/os_list.json bin/config/
	cp config/steps.json bin/config/