all: spotcraft

spotcraft: main.go statik/statik.go
	go build -o spotcraft main.go

statik/statik.go:
	mkdir -p statik
	statik -src=../static-assets

clean:
	rm -f spotcraft
	rm -rf ./statik

.PHONY: all clean static-assets
