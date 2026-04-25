.PHONY: build clean upload push run test

build:
	mkdir -p ./bin
	cd src && go build -ldflags "-s -w" -o ../bin/Tasker .
	upx --best --lzma ./bin/Tasker -o ./bin/Tasker-upx

run:
	cd src && go run .

test:
	cd src && go test ./...

upload:
	scp ./bin/Tasker-upx swim@oracle:/home/swim/tasker/

clean:
	rm -rf ./bin

push: clean build upload